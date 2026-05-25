package s3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultMaxMultipartUploads = 1000
	maxMultipartUploads        = 1000
)

type listMultipartUploadsOptions struct {
	Prefix         string
	KeyMarker      string
	UploadIDMarker string
	MaxUploads     int
}

func (s *Service) handleListMultipartUploads(w http.ResponseWriter, r *http.Request, bucket string) {
	options, err := listMultipartUploadsOptionsFromQuery(r.URL.Query())
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	uploads, err := s.multipart.listBucket(r.Context(), bucket)
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	page, next, truncated := paginateMultipartUploads(filterMultipartUploads(uploads, options), options)
	result := listMultipartUploadsResult{
		XMLNS:              defaultXMLNS,
		Bucket:             bucket,
		Prefix:             options.Prefix,
		KeyMarker:          options.KeyMarker,
		UploadIDMarker:     options.UploadIDMarker,
		MaxUploads:         options.MaxUploads,
		IsTruncated:        truncated,
		Uploads:            make([]multipartUploadItemResult, 0, len(page)),
		NextKeyMarker:      next.Key,
		NextUploadIDMarker: next.UploadID,
	}
	for index := range page {
		upload := page[index]
		result.Uploads = append(result.Uploads, multipartUploadItemResult{
			Key:       upload.Key,
			UploadID:  upload.UploadID,
			Initiated: formatS3Time(upload.CreatedAt),
		})
	}
	s.writeXML(w, http.StatusOK, result)
}

func (m *multipartStore) listBucket(ctx context.Context, bucket string) ([]multipartUpload, error) {
	if err := contextError(ctx, "list multipart uploads"); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, err := m.multipartUploadEntriesLocked()
	if err != nil {
		return nil, err
	}
	uploads, err := m.bucketMultipartUploadsLocked(entries, bucket)
	if err != nil {
		return nil, err
	}
	sortMultipartUploads(uploads)
	return uploads, nil
}

func (m *multipartStore) multipartUploadEntriesLocked() ([]os.DirEntry, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list multipart uploads: %w", err)
	}
	return entries, nil
}

func (m *multipartStore) bucketMultipartUploadsLocked(entries []os.DirEntry, bucket string) ([]multipartUpload, error) {
	uploads := make([]multipartUpload, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		upload, err := m.loadLocked(entry.Name())
		if err != nil {
			if errors.Is(err, errNoSuchUpload) {
				continue
			}
			return nil, err
		}
		if upload.Bucket == bucket {
			uploads = append(uploads, upload)
		}
	}
	return uploads, nil
}

func sortMultipartUploads(uploads []multipartUpload) {
	sort.SliceStable(uploads, func(left, right int) bool {
		if uploads[left].Key == uploads[right].Key {
			return uploads[left].UploadID < uploads[right].UploadID
		}
		return uploads[left].Key < uploads[right].Key
	})
}

func listMultipartUploadsOptionsFromQuery(query map[string][]string) (listMultipartUploadsOptions, error) {
	options := listMultipartUploadsOptions{
		Prefix:         queryValue(query, "prefix"),
		KeyMarker:      queryValue(query, "key-marker"),
		UploadIDMarker: queryValue(query, "upload-id-marker"),
		MaxUploads:     defaultMaxMultipartUploads,
	}
	if maxUploads := queryValue(query, "max-uploads"); maxUploads != "" {
		value, err := strconv.Atoi(maxUploads)
		if err != nil || value < 1 {
			return listMultipartUploadsOptions{}, errInvalidPart
		}
		if value > maxMultipartUploads {
			value = maxMultipartUploads
		}
		options.MaxUploads = value
	}
	return options, nil
}

func filterMultipartUploads(uploads []multipartUpload, options listMultipartUploadsOptions) []multipartUpload {
	filtered := make([]multipartUpload, 0, len(uploads))
	for index := range uploads {
		upload := uploads[index]
		if options.Prefix != "" && !strings.HasPrefix(upload.Key, options.Prefix) {
			continue
		}
		if !multipartUploadAfterMarker(upload, options) {
			continue
		}
		filtered = append(filtered, upload)
	}
	return filtered
}

func multipartUploadAfterMarker(upload multipartUpload, options listMultipartUploadsOptions) bool {
	if options.KeyMarker == "" {
		return true
	}
	if upload.Key > options.KeyMarker {
		return true
	}
	if upload.Key < options.KeyMarker {
		return false
	}
	if options.UploadIDMarker == "" {
		return false
	}
	return upload.UploadID > options.UploadIDMarker
}

func paginateMultipartUploads(
	uploads []multipartUpload,
	options listMultipartUploadsOptions,
) ([]multipartUpload, multipartUpload, bool) {
	if len(uploads) <= options.MaxUploads {
		return uploads, multipartUpload{}, false
	}
	page := uploads[:options.MaxUploads]
	return page, page[len(page)-1], true
}
