package s3

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/lyonbrown4d/maxio/object"
)

type listObjectsOptions struct {
	V2                bool
	Prefix            string
	MaxKeys           int
	ContinuationToken string
	StartAfter        string
	Delimiter         string
}

type listObjectsPage struct {
	Contents              []objectResult
	CommonPrefixes        []commonPrefixResult
	KeyCount              int
	MaxKeys               int
	IsTruncated           bool
	NextContinuationToken string
}

func (s *Service) listObjectsResult(ctx context.Context, bucket string, opts listObjectsOptions) (any, error) {
	objects, err := s.objects.ListObjects(ctx, bucket, opts.Prefix)
	if err != nil {
		return nil, fmt.Errorf("list s3 objects: %w", err)
	}
	if opts.V2 {
		return listObjectsV2Result(bucket, opts, objects), nil
	}
	return listObjectsV1Result(bucket, opts, objects), nil
}

func listObjectsV1Result(bucket string, opts listObjectsOptions, objects []object.ObjectMeta) listBucketResult {
	contents := make([]objectResult, 0, len(objects))
	for i := range objects {
		contents = append(contents, objectResultFromMeta(objects[i]))
	}
	return listBucketResult{
		XMLNS:       defaultXMLNS,
		Name:        bucket,
		Prefix:      opts.Prefix,
		KeyCount:    len(contents),
		MaxKeys:     opts.MaxKeys,
		IsTruncated: false,
		Contents:    contents,
	}
}

func listObjectsV2Result(bucket string, opts listObjectsOptions, objects []object.ObjectMeta) listBucketV2Result {
	page := paginateObjectsV2(opts, objects)
	return listBucketV2Result{
		XMLNS:                 defaultXMLNS,
		Name:                  bucket,
		Prefix:                opts.Prefix,
		Delimiter:             opts.Delimiter,
		KeyCount:              page.KeyCount,
		MaxKeys:               page.MaxKeys,
		IsTruncated:           page.IsTruncated,
		ContinuationToken:     opts.ContinuationToken,
		NextContinuationToken: page.NextContinuationToken,
		StartAfter:            opts.StartAfter,
		Contents:              page.Contents,
		CommonPrefixes:        page.CommonPrefixes,
	}
}

func paginateObjectsV2(opts listObjectsOptions, objects []object.ObjectMeta) listObjectsPage {
	page := listObjectsPage{
		Contents:       make([]objectResult, 0),
		CommonPrefixes: make([]commonPrefixResult, 0),
		MaxKeys:        opts.MaxKeys,
	}
	cursor := listCursor(opts)
	seenPrefixes := make(map[string]struct{})
	lastIncluded := ""
	for i := range objects {
		meta := objects[i]
		included, done := page.addObject(opts, meta, cursor, seenPrefixes)
		if done {
			break
		}
		if included {
			lastIncluded = meta.Key
		}
	}
	if page.IsTruncated && lastIncluded != "" {
		page.NextContinuationToken = encodeContinuationToken(lastIncluded)
	}
	return page
}

type listEntry struct {
	Object       *objectResult
	CommonPrefix *commonPrefixResult
}

func (p *listObjectsPage) addObject(
	opts listObjectsOptions,
	meta object.ObjectMeta,
	cursor string,
	seenPrefixes map[string]struct{},
) (bool, bool) {
	if cursor != "" && meta.Key <= cursor {
		return false, false
	}
	entry, ok := listEntryForObject(opts, meta, seenPrefixes)
	if !ok {
		return false, false
	}
	if p.MaxKeys == 0 || p.KeyCount >= p.MaxKeys {
		p.IsTruncated = true
		return false, true
	}
	p.add(entry)
	return true, false
}

func listEntryForObject(
	opts listObjectsOptions,
	meta object.ObjectMeta,
	seenPrefixes map[string]struct{},
) (listEntry, bool) {
	if opts.Delimiter == "" {
		result := objectResultFromMeta(meta)
		return listEntry{Object: &result}, true
	}
	remaining := strings.TrimPrefix(meta.Key, opts.Prefix)
	if delimiterIndex := strings.Index(remaining, opts.Delimiter); delimiterIndex >= 0 {
		prefix := opts.Prefix + remaining[:delimiterIndex+len(opts.Delimiter)]
		if _, ok := seenPrefixes[prefix]; ok {
			return listEntry{}, false
		}
		seenPrefixes[prefix] = struct{}{}
		return listEntry{CommonPrefix: &commonPrefixResult{Prefix: prefix}}, true
	}
	result := objectResultFromMeta(meta)
	return listEntry{Object: &result}, true
}

func (p *listObjectsPage) add(entry listEntry) {
	if entry.Object != nil {
		p.Contents = append(p.Contents, *entry.Object)
		p.KeyCount++
		return
	}
	if entry.CommonPrefix != nil {
		p.CommonPrefixes = append(p.CommonPrefixes, *entry.CommonPrefix)
		p.KeyCount++
	}
}

func objectResultFromMeta(meta object.ObjectMeta) objectResult {
	return objectResult{
		Key:          meta.Key,
		LastModified: formatS3Time(meta.UpdatedAt),
		ETag:         meta.ETag,
		Size:         meta.Size,
		StorageClass: "STANDARD",
	}
}

func listCursor(opts listObjectsOptions) string {
	if opts.ContinuationToken != "" {
		return decodeContinuationToken(opts.ContinuationToken)
	}
	return opts.StartAfter
}

func encodeContinuationToken(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

func decodeContinuationToken(token string) string {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return token
	}
	return string(decoded)
}

func listObjectsOptionsFromQuery(query url.Values) listObjectsOptions {
	return listObjectsOptions{
		V2:                query.Get("list-type") == "2",
		Prefix:            query.Get("prefix"),
		MaxKeys:           maxKeysV2(query.Get("max-keys")),
		ContinuationToken: query.Get("continuation-token"),
		StartAfter:        query.Get("start-after"),
		Delimiter:         query.Get("delimiter"),
	}
}

func listObjectsOptionsFromHTTPX(input *httpxBucketInput) listObjectsOptions {
	return listObjectsOptions{
		V2:                input.ListType == "2",
		Prefix:            input.Prefix,
		MaxKeys:           normalizeHTTPXMaxKeys(input.MaxKeys),
		ContinuationToken: input.ContinuationToken,
		StartAfter:        input.StartAfter,
		Delimiter:         input.Delimiter,
	}
}

func maxKeysV2(value string) int {
	if value == "" {
		return 1000
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 1000
	}
	return parsed
}
