package processing

import (
	"encoding/json"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/mo"
)

func objectKey(object ObjectRef) string {
	version := object.VersionID
	if version == "" {
		version = object.Digest
	}
	return object.Bucket + "\x00" + object.Key + "\x00" + version
}

func modelFromRecord(record Record) model.ProcessingRecord {
	return model.ProcessingRecord{
		Bucket:    record.Object.Bucket,
		Key:       record.Object.Key,
		VersionID: record.Object.VersionID,
		Digest:    record.Object.Digest,
		Mode:      record.Mode,
		Status:    record.Status,
		Error:     record.Error,
		Results:   marshalResults(record.Results),
		UpdatedAt: record.UpdatedAt,
	}
}

func recordFromModel(record model.ProcessingRecord) Record {
	return Record{
		Object:    ObjectRef{Bucket: record.Bucket, Key: record.Key, VersionID: record.VersionID, Digest: record.Digest},
		Mode:      record.Mode,
		Status:    record.Status,
		Error:     record.Error,
		Results:   unmarshalResults(record.Results),
		UpdatedAt: record.UpdatedAt,
	}
}

func marshalResults(results *collectionlist.List[ProcessorResult]) string {
	if results == nil {
		return "[]"
	}
	data := mo.TupleToResult(json.Marshal(results.Values())).OrElse([]byte("[]"))
	return string(data)
}

func unmarshalResults(raw string) *collectionlist.List[ProcessorResult] {
	items := []ProcessorResult{}
	if strings.TrimSpace(raw) == "" {
		return collectionlist.NewList(items...)
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return collectionlist.NewList[ProcessorResult]()
	}
	return collectionlist.NewList(items...)
}
