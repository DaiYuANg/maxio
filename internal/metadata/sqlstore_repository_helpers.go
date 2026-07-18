package metadata

import (
	"context"
	"fmt"

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
