package metadata

import (
	"context"
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
)

func getRepositoryByKey[E any, S repositoryx.EntitySchema[E]](
	ctx context.Context,
	repository *repositoryx.Base[E, S],
	key repositoryx.TypedKeySet,
	operation string,
) (E, bool, error) {
	var zero E
	if repository == nil {
		return zero, false, fmt.Errorf("%s: metadata repository is nil", operation)
	}
	option, err := repository.GetByKeySetOption(ctx, key)
	if err != nil {
		return zero, false, fmt.Errorf("%s: %w", operation, err)
	}
	entity, found := option.Get()
	return entity, found, nil
}

func deleteRepositoryByKey[E any, S repositoryx.EntitySchema[E]](
	ctx context.Context,
	repository *repositoryx.Base[E, S],
	key repositoryx.TypedKeySet,
	operation string,
) (bool, error) {
	if repository == nil {
		return false, fmt.Errorf("%s: metadata repository is nil", operation)
	}
	result, err := repository.DeleteByKeySet(ctx, key)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return hasAffectedRow(result, operation)
}

func repositoryInsertAssignments[E any, S repositoryx.EntitySchema[E]](
	ctx context.Context,
	repository *repositoryx.Base[E, S],
	schema S,
	entity *E,
	operation string,
) (*collectionlist.List[querydsl.Assignment], error) {
	if repository == nil {
		return nil, fmt.Errorf("%s: metadata repository is nil", operation)
	}
	assignments, err := repository.Mapper().InsertAssignmentsWithID(ctx, schema, entity, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return assignments, nil
}

func requireStoredEntity[E any](entity E, found bool, err error) (E, error) {
	var zero E
	if err != nil {
		return zero, err
	}
	if !found {
		return zero, ErrObjectNotFound
	}
	return entity, nil
}

func repositorySpecs(specs ...repositoryx.Spec) []repositoryx.Spec {
	return collectionlist.FilterMapList(
		collectionlist.NewList(specs...),
		func(_ int, spec repositoryx.Spec) (repositoryx.Spec, bool) {
			return spec, spec != nil
		},
	).Values()
}

func optionalWhereSpec(predicate querydsl.Predicate) repositoryx.Spec {
	if predicate == nil {
		return nil
	}
	return repositoryx.Where(predicate)
}
