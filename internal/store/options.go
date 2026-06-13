package store

import (
	"maps"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

// PutOptions captures protocol-level object metadata that is persisted in the
// metadata layer. The engine layout intentionally stays focused on shard
// placement and content addressing.
type PutOptions struct {
	ContentType        string
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	ContentLanguage    string
	UserMetadata       map[string]string
}

func (opts PutOptions) normalized() PutOptions {
	opts.ContentType = strings.TrimSpace(opts.ContentType)
	opts.CacheControl = strings.TrimSpace(opts.CacheControl)
	opts.ContentDisposition = strings.TrimSpace(opts.ContentDisposition)
	opts.ContentEncoding = strings.TrimSpace(opts.ContentEncoding)
	opts.ContentLanguage = strings.TrimSpace(opts.ContentLanguage)
	opts.UserMetadata = normalizeUserMetadata(opts.UserMetadata)
	return opts
}

func (opts PutOptions) apply(meta model.ObjectMeta) model.ObjectMeta {
	meta.ContentType = opts.ContentType
	meta.CacheControl = opts.CacheControl
	meta.ContentDisposition = opts.ContentDisposition
	meta.ContentEncoding = opts.ContentEncoding
	meta.ContentLanguage = opts.ContentLanguage
	meta.UserMetadata = cloneUserMetadata(opts.UserMetadata)
	return meta
}

func normalizeUserMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		output[key] = value
	}
	if len(output) == 0 {
		return nil
	}
	return output
}

func cloneUserMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	maps.Copy(output, input)
	return output
}
