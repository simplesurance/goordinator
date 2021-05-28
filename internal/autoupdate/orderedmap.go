package autoupdate

import "container/list"

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

// EnqueueIfNotExist appends an element to the map if it does not exist already.
func (m *orderedMap) EnqueueIfNotExist(key int, val *PullRequest) (newFirstElem *PullRequest, existed bool) {
	if _, exist := m.m[key]; exist {
		return nil, false
	}

	elem := m.order.PushBack(val)
	m.m[key] = elem

	if m.order.Len() == 1 {
		return val, true
	}

	return nil, false
}

func (m *orderedMap) Get(key int) *PullRequest {
	v, exist := m.m[key]
	if !exist {
		return nil
	}

	return v.Value.(*PullRequest)
}

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
// If the map is empty, nil is returned
func (m *orderedMap) First() *PullRequest {
	if e := m.order.Front(); e != nil {
		return e.Value.(*PullRequest)
	}

	return nil
}

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
