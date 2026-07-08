package processing

import (
	"context"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *Service) Discard(ctx context.Context, object ObjectRef) {
	if s == nil {
		return
	}
	ctx = contextOrBackground(ctx)
	if object.VersionID != "" {
		s.discardVersion(ctx, object)
		return
	}
	s.discardDigest(ctx, object)
}

func (s *Service) discardVersion(ctx context.Context, object ObjectRef) {
	key := objectKey(object)
	s.mu.Lock()
	s.rememberDiscardedLocked(key)
	delete(s.records, key)
	s.mu.Unlock()
	s.deleteRecordFromStore(ctx, object)
}

func (s *Service) discardDigest(ctx context.Context, object ObjectRef) {
	key := objectKey(object)
	s.mu.Lock()
	delete(s.records, key)
	s.mu.Unlock()
	s.deleteRecordFromStore(ctx, object)
}

func (s *Service) deleteRecordFromStore(ctx context.Context, object ObjectRef) {
	if s.store == nil {
		return
	}
	if _, err := s.store.DeleteProcessingRecord(ctx, object.Bucket, object.Key, object.VersionID, object.Digest); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "delete processing record", "bucket", object.Bucket, "key", object.Key, "version_id", object.VersionID, "error", err)
	}
}

func (s *Service) Record(ctx context.Context, object ObjectRef) (Record, bool) {
	record, found, err := s.LookupRecord(ctx, object)
	if err != nil {
		return Record{}, false
	}
	return record, found
}

func (s *Service) LookupRecord(ctx context.Context, object ObjectRef) (Record, bool, error) {
	return s.lookupRecord(ctx, object)
}

func (s *Service) ListRecords(ctx context.Context, status string, limit int) (*collectionlist.List[Record], error) {
	if s == nil {
		return collectionlist.NewList[Record](), nil
	}
	ctx = contextOrBackground(ctx)
	status = NormalizeStatus(status)
	limit = normalizeRecordListLimit(limit)
	if s.store != nil {
		return s.listStoredRecords(ctx, status, limit)
	}
	return s.listMemoryRecords(status, limit), nil
}

func (s *Service) listStoredRecords(ctx context.Context, status string, limit int) (*collectionlist.List[Record], error) {
	stored, err := s.store.ListProcessingRecords(ctx, status, limit+s.discardedTombstoneCount())
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "list processing records", "status", status, "limit", limit, "error", err)
		}
		return nil, fmt.Errorf("list processing records: %w", err)
	}
	if stored == nil {
		return collectionlist.NewList[Record](), nil
	}
	return s.filterStoredRecords(stored, limit), nil
}

func (s *Service) filterStoredRecords(stored *collectionlist.List[model.ProcessingRecord], limit int) *collectionlist.List[Record] {
	records := collectionlist.NewListWithCapacity[Record](stored.Len())
	stored.Range(func(_ int, storedRecord model.ProcessingRecord) bool {
		record := recordFromModel(storedRecord)
		if !s.isDiscarded(record.Object) {
			records.Add(record)
		}
		return records.Len() < limit
	})
	return records
}

func (s *Service) listMemoryRecords(status string, limit int) *collectionlist.List[Record] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := collectionlist.NewListWithCapacity[Record](len(s.records))
	for key := range s.records {
		record := s.records[key]
		if status == "" || record.Status == status {
			records.Add(record)
		}
	}
	return limitRecords(sortRecords(records), limit)
}

func sortRecords(records *collectionlist.List[Record]) *collectionlist.List[Record] {
	return records.Sort(func(left, right Record) int {
		if left.UpdatedAt.After(right.UpdatedAt) {
			return -1
		}
		if left.UpdatedAt.Before(right.UpdatedAt) {
			return 1
		}
		return strings.Compare(objectKey(left.Object), objectKey(right.Object))
	})
}

func limitRecords(records *collectionlist.List[Record], limit int) *collectionlist.List[Record] {
	if records.Len() <= limit {
		return records
	}
	return collectionlist.NewList(records.Values()[:limit]...)
}

func (s *Service) lookupRecord(ctx context.Context, object ObjectRef) (Record, bool, error) {
	if s == nil || s.isDiscarded(object) {
		return Record{}, false, nil
	}
	ctx = contextOrBackground(ctx)
	if s.store != nil {
		return s.lookupStoredRecord(ctx, object)
	}
	return s.lookupMemoryRecord(object)
}

func (s *Service) lookupStoredRecord(ctx context.Context, object ObjectRef) (Record, bool, error) {
	stored, found, err := s.store.GetProcessingRecord(ctx, object.Bucket, object.Key, object.VersionID, object.Digest)
	if s.isDiscarded(object) {
		return Record{}, false, nil
	}
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "get processing record", "bucket", object.Bucket, "key", object.Key, "version_id", object.VersionID, "error", err)
		}
		return Record{}, false, fmt.Errorf("get processing record: %w", err)
	}
	if !found {
		return Record{}, false, nil
	}
	return recordFromModel(stored), true, nil
}

func (s *Service) lookupMemoryRecord(object ObjectRef) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, found := s.records[objectKey(object)]
	return record, found, nil
}

func (s *Service) discardDigestRecord(ctx context.Context, object ObjectRef) {
	if object.Digest == "" {
		return
	}
	digestObject := object
	digestObject.VersionID = ""
	s.Discard(ctx, digestObject)
}

func (s *Service) promotableDigestRecord(ctx context.Context, object ObjectRef) (Record, bool) {
	if object.VersionID == "" || object.Digest == "" {
		return Record{}, false
	}
	digestObject := object
	digestObject.VersionID = ""
	record, found, err := s.lookupRecord(ctx, digestObject)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(contextOrBackground(ctx), "get digest processing record", "bucket", object.Bucket, "key", object.Key, "digest", object.Digest, "error", err)
		}
		return Record{}, false
	}
	return record, found
}

func (s *Service) storeRecord(ctx context.Context, object ObjectRef, status, errorText string, results *collectionlist.List[ProcessorResult]) error {
	if s == nil {
		return nil
	}
	ctx = contextOrBackground(ctx)
	key := objectKey(object)
	if s.isDiscarded(object) {
		return nil
	}
	record := Record{Object: object, Mode: s.cfg.Mode, Status: status, Error: errorText, Results: results, UpdatedAt: time.Now().UTC()}
	storeErr := s.upsertStoredRecord(ctx, record)
	s.mu.Lock()
	if _, discarded := s.discarded[key]; discarded {
		s.mu.Unlock()
		s.deleteRecordFromStore(ctx, object)
		return storeErr
	}
	s.records[key] = record
	s.mu.Unlock()
	return storeErr
}

func (s *Service) upsertStoredRecord(ctx context.Context, record Record) error {
	if s.store == nil {
		return nil
	}
	if _, err := s.store.UpsertProcessingRecord(ctx, modelFromRecord(record)); err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "upsert processing record", "bucket", record.Object.Bucket, "key", record.Object.Key, "version_id", record.Object.VersionID, "error", err)
		}
		return fmt.Errorf("upsert processing record: %w", err)
	}
	return nil
}

func (s *Service) storeRecordOrWarn(ctx context.Context, object ObjectRef, status, errorText string, results *collectionlist.List[ProcessorResult], message string) {
	if err := s.storeRecord(ctx, object, status, errorText, results); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, message, "bucket", object.Bucket, "key", object.Key, "version_id", object.VersionID, "error", err)
	}
}

func (s *Service) discardedTombstoneCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.discardedOrder)
}

func (s *Service) rememberDiscardedLocked(key string) {
	if _, exists := s.discarded[key]; exists {
		return
	}
	s.discarded[key] = struct{}{}
	s.discardedOrder = append(s.discardedOrder, key)
	if len(s.discardedOrder) <= maxDiscardedTombstones {
		return
	}
	oldest := s.discardedOrder[0]
	s.discardedOrder = s.discardedOrder[1:]
	delete(s.discarded, oldest)
}

func (s *Service) isDiscarded(object ObjectRef) bool {
	if object.VersionID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, discarded := s.discarded[objectKey(object)]
	return discarded
}
