package s3

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/lyonbrown4d/maxio/object"
)

func (s *Service) handleGetBucketLocation(w http.ResponseWriter, r *http.Request, bucket string) {
	if _, err := s.objects.ListObjects(r.Context(), bucket, ""); err != nil {
		s.writeMappedError(w, err)
		return
	}
	s.writeXML(w, http.StatusOK, bucketLocationResult{
		XMLNS:    defaultXMLNS,
		Location: s.region(),
	})
}

func (s *Service) handleDeleteObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	request, err := decodeDeleteObjectsRequest(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "MalformedXML", err.Error())
		return
	}
	result := deleteObjectsResult{
		XMLNS:   defaultXMLNS,
		Deleted: make([]deletedObjectResult, 0, len(request.Objects)),
	}
	for index := range request.Objects {
		target := request.Objects[index]
		item := s.deleteObjectItem(r, bucket, strings.TrimSpace(target.Key))
		if item.err != nil {
			result.Errors = append(result.Errors, item.err.toResult())
			continue
		}
		if !request.Quiet {
			result.Deleted = append(result.Deleted, deletedObjectResult{Key: item.key})
		}
	}
	s.writeXML(w, http.StatusOK, result)
}

func decodeDeleteObjectsRequest(r *http.Request) (deleteObjectsRequest, error) {
	request := deleteObjectsRequest{}
	if r.Body == nil {
		return request, errors.New("delete request body is required")
	}
	if err := xml.NewDecoder(r.Body).Decode(&request); err != nil {
		return request, fmt.Errorf("decode delete request: %w", err)
	}
	if len(request.Objects) == 0 {
		return request, errors.New("delete request must contain at least one object")
	}
	return request, nil
}

type deleteObjectItemResult struct {
	key string
	err *deleteObjectItemError
}

type deleteObjectItemError struct {
	key     string
	code    string
	message string
}

func (s *Service) deleteObjectItem(
	r *http.Request,
	bucket string,
	key string,
) deleteObjectItemResult {
	if key == "" {
		return deleteObjectItemResult{err: &deleteObjectItemError{
			key:     key,
			code:    "InvalidArgument",
			message: "object key is required",
		}}
	}
	if _, err := s.objects.DeleteObject(r.Context(), bucket, key); err != nil {
		if errors.Is(err, object.ErrNotFound) {
			return deleteObjectItemResult{key: key}
		}
		status, code := mapError(err)
		if status == http.StatusNotFound && errors.Is(err, object.ErrBucketNotFound) {
			return deleteObjectItemResult{err: &deleteObjectItemError{
				key:     key,
				code:    code,
				message: err.Error(),
			}}
		}
		return deleteObjectItemResult{err: &deleteObjectItemError{
			key:     key,
			code:    code,
			message: err.Error(),
		}}
	}
	return deleteObjectItemResult{key: key}
}

func (err *deleteObjectItemError) toResult() deleteErrorResult {
	return deleteErrorResult{
		Key:     err.key,
		Code:    err.code,
		Message: err.message,
	}
}

func (s *Service) region() string {
	region := strings.TrimSpace(s.cfg.Region)
	if region == "" {
		return "us-east-1"
	}
	return region
}
