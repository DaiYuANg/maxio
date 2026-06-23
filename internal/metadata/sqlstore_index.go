package metadata

import (
	"context"
	"fmt"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/model"
	"strings"
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
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
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

	option, err := s.repos.indexDocuments.GetByKeySetOption(ctx, repositoryx.KeySet(repositoryx.Part(metadataIndexDocuments.id, id)))
	if err != nil {
		return model.IndexDocument{}, false, fmt.Errorf("query index document: %w", err)
	}
	document, found := option.Get()
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
	documents, err := s.repos.indexDocuments.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list index documents: %w", err)
	}
	return documents, nil
}

func indexDocumentFilter(bucket, prefix string) querydsl.Predicate {
	predicates := collectionlist.NewList[querydsl.Predicate]()
	if bucket != "" {
		predicates.Add(metadataIndexDocuments.bucket.Eq(bucket))
	}
	if prefix != "" {
		predicates.Add(querydsl.Like(metadataIndexDocuments.key, prefixPattern(prefix)))
	}
	if predicates.IsEmpty() {
		return nil
	}
	return querydsl.AndList(querydsl.CompactPredicatesList(predicates))
}

func (s *SQLMetadata) DeleteIndexDocument(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	result, err := s.repos.indexDocuments.DeleteByKeySet(ctx, repositoryx.KeySet(repositoryx.Part(metadataIndexDocuments.id, id)))
	if err != nil {
		return false, fmt.Errorf("delete index document: %w", err)
	}
	return hasAffectedRow(result, "delete index document")
}
