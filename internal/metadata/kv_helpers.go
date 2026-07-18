package metadata

import (
	"github.com/arcgolabs/collectionx/list"
	collectionmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/lo"
)

func listKeysFromMap[K comparable, V any](values map[K]V) *list.List[K] {
	return list.NewList(collectionmapping.NewMapFrom(values).Keys()...)
}

func listValuesFromMap[K comparable, V any](values map[K]V) *list.List[V] {
	return list.NewList(collectionmapping.NewMapFrom(values).Values()...)
}

func listValuesFromMapWithKey[V any, T any](
	values map[string]V,
	mapper func(string, V) T,
) *list.List[T] {
	return list.NewList(lo.MapToSlice(values, mapper)...)
}
