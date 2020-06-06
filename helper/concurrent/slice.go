package concurrent

import "sync"

type Slice struct {
	sync.RWMutex
	items []interface{}
}

// concurrent slice item
type SliceItem struct {
	Index int
	Value interface{}
}
func (cs *Slice) Append(item interface{}) {
	cs.Lock()
	defer cs.Unlock()

	cs.items = append(cs.items, item)
}
func (cs *Slice) Iter() <-chan SliceItem {
	c := make(chan SliceItem)

	f := func() {
		cs.Lock()
		defer cs.Unlock()
		for index, value := range cs.items {
			c <- SliceItem{index, value}
		}
		close(c)
	}
	go f()

	return c
}