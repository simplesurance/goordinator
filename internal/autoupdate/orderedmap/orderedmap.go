package orderedmap

import (
	"github.com/simplesurance/goordinator/internal/linkedlist"
)

// Map is a map datastructure that allows accessing it's element in a
// fixed order.
type Map[K comparable, V any] struct {
	order   *linkedlist.List[V]
	m       map[K]*linkedlist.Element[V]
	zeroval V
}

func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		order: linkedlist.New[V](),
		m:     map[K]*linkedlist.Element[V]{},
	}
}

// EnqueueIfNotExist adds val to the map if K does not exist.
func (m *Map[K, V]) EnqueueIfNotExist(key K, val V) (isFirst, added bool) {
	if _, exist := m.m[key]; exist {
		return false, false
	}

	elem := m.order.PushBack(val)
	m.m[key] = elem

	return m.order.Len() == 1, true
}

// Get returns the value for the given key.
// If the key does not exist, the zero value is returned
func (m *Map[K, V]) Get(key K) V {
	v, exist := m.m[key]
	if !exist {
		return m.zeroval
	}

	return v.Value
}

// Dequeue removes the value with the key from the map and returns it.
// If thekey does not exist in the map, the zero value is returned.
func (m *Map[K, V]) Dequeue(key K) (removedElem V) {
	v, exist := m.m[key]
	if !exist {
		return m.zeroval
	}
	delete(m.m, key)

	return m.order.Remove(v)
}

// First returns the first element in the map.
// If the map is empty, the zero value is returned.
func (m *Map[K, V]) First() V {
	if e := m.order.Front(); e != nil {
		return e.Value
	}

	return m.zeroval
}

// Len returns the number of elements in the maps.
func (m *Map[K, V]) Len() int {
	return m.order.Len()
}

// Foreach itereates through the map in order.
// When fn returns false the iteration is aborted.
func (m *Map[K, V]) Foreach(fn func(V) bool) {
	for e := m.order.Front(); e != nil; e = e.Next() {
		if !fn(e.Value) {
			return
		}
	}
}

// AsSlice returns a new slice containing the elements of the orderedMap in
// order.
func (m *Map[K, V]) AsSlice() []V {
	result := make([]V, 0, m.order.Len())

	for e := m.order.Front(); e != nil; e = e.Next() {
		result = append(result, e.Value)
	}

	return result
}
