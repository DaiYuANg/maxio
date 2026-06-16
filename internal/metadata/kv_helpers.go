package metadata

import "github.com/arcgolabs/collectionx/list"
import collectionmapping "github.com/arcgolabs/collectionx/mapping"

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
	source := collectionmapping.NewMapFrom(values)
	items := list.NewListWithCapacity[T](source.Len())
	source.Range(func(key string, value V) bool {
		items.Add(mapper(key, value))
		return true
	})
	return items
}
