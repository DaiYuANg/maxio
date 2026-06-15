package metadata

import (
	"context"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertIndexDocument(ctx context.Context, document model.IndexDocument) (model.IndexDocument, error) {
	document, err := prepareIndexDocument(document)
	if err != nil {
		return model.IndexDocument{}, err
	}
	query := querydsl.InsertInto(metadataIndexDocuments.schema).
		Values(
			metadataIndexDocuments.id.Set(document.ID),
			metadataIndexDocuments.bucket.Set(document.Bucket),
			metadataIndexDocuments.key.Set(document.Key),
			metadataIndexDocuments.versionID.Set(document.VersionID),
			metadataIndexDocuments.digest.Set(document.Digest),
			metadataIndexDocuments.state.Set(document.State),
			metadataIndexDocuments.errorText.Set(document.Error),
			metadataIndexDocuments.indexedAt.Set(unixNanoOrNil(document.IndexedAt)),
			metadataIndexDocuments.createdAt.Set(document.CreatedAt.UnixNano()),
			metadataIndexDocuments.updatedAt.Set(document.UpdatedAt.UnixNano()),
		).
		OnConflict(metadataIndexDocuments.id).
		DoUpdateSet(
			metadataIndexDocuments.bucket.SetExcluded(),
			metadataIndexDocuments.key.SetExcluded(),
			metadataIndexDocuments.versionID.SetExcluded(),
			metadataIndexDocuments.digest.SetExcluded(),
			metadataIndexDocuments.state.SetExcluded(),
			metadataIndexDocuments.errorText.SetExcluded(),
			metadataIndexDocuments.indexedAt.SetExcluded(),
			metadataIndexDocuments.updatedAt.SetExcluded(),
		)
	if _, execErr := s.execBuilderContext(ctx, query); execErr != nil {
		return model.IndexDocument{}, fmt.Errorf("upsert index document: %w", execErr)
	}
	stored, found, err := s.GetIndexDocument(ctx, document.ID)
	if err != nil {
		return model.IndexDocument{}, err
	}
	if !found {
		return model.IndexDocument{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLMetadata) GetIndexDocument(ctx context.Context, id string) (model.IndexDocument, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexDocument{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataIndexDocuments.schema, metadataIndexDocuments.selectItems()...).
		Where(metadataIndexDocuments.id.Eq(id)).
		Limit(1)
	document, found, err := querySQLOne(ctx, s, query, "index document", metadataIndexDocumentMapper)
	if err != nil {
		return model.IndexDocument{}, false, err
	}
	return document, found, nil
}

func (s *SQLMetadata) ListIndexDocuments(ctx context.Context, bucket, prefix string) (*collectionlist.List[model.IndexDocument], error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	query := querydsl.SelectFrom(metadataIndexDocuments.schema, metadataIndexDocuments.selectItems()...).
		OrderBy(metadataIndexDocuments.bucket.Asc(), metadataIndexDocuments.key.Asc(), metadataIndexDocuments.versionID.Asc())
	if predicate := indexDocumentFilter(bucket, prefix); predicate != nil {
		query.Where(predicate)
	}
	documents, err := querySQLRows(
		ctx,
		s,
		query,
		"index documents",
		metadataIndexDocumentMapper,
	)
	if err != nil {
		return nil, err
	}
	return documents, nil
}

func indexDocumentFilter(bucket, prefix string) querydsl.Predicate {
	bucketFilter := metadataIndexDocuments.bucket.Eq(bucket)
	prefixFilter := querydsl.Like(metadataIndexDocuments.key, prefixPattern(prefix))
	switch {
	case bucket != "" && prefix != "":
		return querydsl.And(bucketFilter, prefixFilter)
	case bucket != "":
		return bucketFilter
	case prefix != "":
		return prefixFilter
	default:
		return nil
	}
}

func (s *SQLMetadata) DeleteIndexDocument(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	query := querydsl.DeleteFrom(metadataIndexDocuments.schema).Where(metadataIndexDocuments.id.Eq(id))
	result, err := s.execBuilderContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete index document: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete index document rows: %w", err)
	}
	return affected > 0, nil
}
