package autoupdate

import "container/list"

// orderedMap is a map datastructure that allows accessing it's element in a
// fixed order.
type orderedMap struct {
	order *list.List
	m     map[int]*list.Element
}

func newOrderedMap() *orderedMap {
	return &orderedMap{
		order: list.New(),
		m:     map[int]*list.Element{},
	}
}

// EnqueueIfNotExist appends an element to the map if it is not in the map
// already.
// The method panics if val is nil.
func (m *orderedMap) EnqueueIfNotExist(key int, val *PullRequest) (newFirstElem *PullRequest, added bool) {
	if val == nil {
		panic("pullrequest is nil")
	}

	if _, exist := m.m[key]; exist {
		return nil, false
	}

	elem := m.order.PushBack(val)
	m.m[key] = elem

	if m.order.Len() == 1 {
		return val, true
	}

	return nil, true
}

// Get returns the value for the given key.
// If the key does not exist, nil is returned.
func (m *orderedMap) Get(key int) *PullRequest {
	v, exist := m.m[key]
	if !exist {
		return nil
	}

	return v.Value.(*PullRequest)
}

// Dequeue removes the value with the key from the map.
func (m *orderedMap) Dequeue(key int) (removedElem, newFirstElem *PullRequest) {
	v, exist := m.m[key]
	if !exist {
		return nil, nil
	}

	delete(m.m, key)

	firstElem := m.order.Front()
	removedElem = m.order.Remove(v).(*PullRequest)

	if !firstElem.Value.(*PullRequest).Equal(removedElem) {
		return removedElem, nil
	}

	return removedElem, m.First()
}

// First returns the first element in the map.
// If the map is empty, nil is returned.
func (m *orderedMap) First() *PullRequest {
	if e := m.order.Front(); e != nil {
		return e.Value.(*PullRequest)
	}

	return nil
}

// Len returns the number of elements in the maps.
func (m *orderedMap) Len() int {
	return m.order.Len()
}

// Foreach itereates through the map in order.
// When fn returns false the iteration is aborted.
func (m *orderedMap) Foreach(fn func(*PullRequest) bool) {
	for e := m.order.Front(); e != nil; e = e.Next() {
		if !fn(e.Value.(*PullRequest)) {
			return
		}
	}
}
