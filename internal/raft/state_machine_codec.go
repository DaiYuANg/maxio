package raft

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	dbsm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/lyonbrown4d/maxio/model"
)

func encodeMetadataEnvelope(envelope metadataEnvelope) (dbsm.Result, error) {
	data, err := json.Marshal(envelope)
	if err != nil {
		return dbsm.Result{}, fmt.Errorf("marshal metadata result: %w", err)
	}
	return dbsm.Result{Data: data}, nil
}

func decodeMetadataEnvelope(data []byte) (MetadataResult, error) {
	if len(data) == 0 {
		return MetadataResult{}, nil
	}
	var envelope metadataEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return MetadataResult{}, fmt.Errorf("unmarshal metadata result: %w", err)
	}
	if envelope.Error != "" {
		return envelope.Result, metadataError(envelope.Error)
	}
	return envelope.Result, nil
}

func resultFromMetadataEnvelope(value any) (MetadataResult, error) {
	switch typed := value.(type) {
	case MetadataResult:
		return typed, nil
	case metadataEnvelope:
		if typed.Error != "" {
			return typed.Result, metadataError(typed.Error)
		}
		return typed.Result, nil
	case *metadataEnvelope:
		if typed == nil {
			return MetadataResult{}, metadataError(MetadataErrorBadRequest)
		}
		if typed.Error != "" {
			return typed.Result, metadataError(typed.Error)
		}
		return typed.Result, nil
	default:
		return MetadataResult{}, metadataError(MetadataErrorBadRequest)
	}
}

type metadataError string

func (e metadataError) Error() string {
	return string(e)
}

func MetadataErrorCode(err error) string {
	var code metadataError
	if errors.As(err, &code) {
		return string(code)
	}
	return ""
}

func invalidName(value string) bool {
	return strings.TrimSpace(value) == ""
}

func invalidObject(bucket, key string) bool {
	return invalidName(bucket) || strings.TrimSpace(key) == ""
}

func objectMapKey(bucket, key string) string {
	return bucket + "\x00" + key
}

func copyBucketMap(input map[string]model.Bucket) map[string]model.Bucket {
	output := make(map[string]model.Bucket, len(input))
	maps.Copy(output, input)
	return output
}

func copyObjectMap(input map[string]model.ObjectMeta) map[string]model.ObjectMeta {
	output := make(map[string]model.ObjectMeta, len(input))
	maps.Copy(output, input)
	return output
}

func copyBlobRefMap(input map[string]MetadataBlobRef) map[string]MetadataBlobRef {
	output := make(map[string]MetadataBlobRef, len(input))
	maps.Copy(output, input)
	return output
}

func cloneRaftBlobChecksums(checksums []string) []string {
	if len(checksums) == 0 {
		return nil
	}
	output := make([]string, len(checksums))
	copy(output, checksums)
	return output
}
