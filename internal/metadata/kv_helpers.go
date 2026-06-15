package metadata

import "github.com/arcgolabs/collectionx/list"

func listKeysFromMap[K comparable, V any](values map[K]V) *list.List[K] {
	keys := list.NewListWithCapacity[K](len(values))
	for key := range values {
		keys.Add(key)
	}
	return keys
}

func listValuesFromMap[K comparable, V any](values map[K]V) *list.List[V] {
	items := list.NewListWithCapacity[V](len(values))
	for _, value := range values {
		items.Add(value)
	}
	return items
}

func listValuesFromMapWithKey[V any, T any](
	values map[string]V,
	mapper func(string, V) T,
) *list.List[T] {
	items := list.NewListWithCapacity[T](len(values))
	for key, value := range values {
		items.Add(mapper(key, value))
	}
	return items
}
