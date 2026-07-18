package metadata

import (
	"context"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertIndexDocument(ctx context.Context, document model.IndexDocument) (model.IndexDocument, error) {
	document, err := prepareIndexDocument(document)
	if err != nil {
		return model.IndexDocument{}, err
	}
	assignments := collectionlist.NewList[querydsl.Assignment](
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
	)
	if err := execUpsertAssignments(
		ctx,
		s.dbxDB,
		metadataIndexDocuments.schema,
		assignments,
		"upsert index document",
		collectionlist.NewList[querydsl.Expression](metadataIndexDocuments.id),
		metadataIndexDocuments.bucket.SetExcluded(),
		metadataIndexDocuments.key.SetExcluded(),
		metadataIndexDocuments.versionID.SetExcluded(),
		metadataIndexDocuments.digest.SetExcluded(),
		metadataIndexDocuments.state.SetExcluded(),
		metadataIndexDocuments.errorText.SetExcluded(),
		metadataIndexDocuments.indexedAt.SetExcluded(),
		metadataIndexDocuments.updatedAt.SetExcluded(),
	); err != nil {
		return model.IndexDocument{}, err
	}
	return requireStoredEntity(s.GetIndexDocument(ctx, document.ID))
}

func (s *SQLMetadata) GetIndexDocument(ctx context.Context, id string) (model.IndexDocument, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexDocument{}, false, ErrBadRequest
	}

	return getRepositoryByKey[model.IndexDocument](
		ctx,
		s.repos.indexDocuments,
		repositoryx.KeySet(repositoryx.Part(metadataIndexDocuments.id, id)),
		"query index document",
	)
}

func (s *SQLMetadata) ListIndexDocuments(ctx context.Context, bucket, prefix string) (*collectionlist.List[model.IndexDocument], error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	specs := repositorySpecs(
		optionalWhereSpec(indexDocumentFilter(bucket, prefix)),
		repositoryx.OrderBy(metadataIndexDocuments.bucket.Asc(), metadataIndexDocuments.key.Asc(), metadataIndexDocuments.versionID.Asc()),
	)
	documents, err := s.repos.indexDocuments.ListSpec(ctx, specs...)
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
	return deleteRepositoryByKey[model.IndexDocument](
		ctx,
		s.repos.indexDocuments,
		repositoryx.KeySet(repositoryx.Part(metadataIndexDocuments.id, id)),
		"delete index document",
	)
}
