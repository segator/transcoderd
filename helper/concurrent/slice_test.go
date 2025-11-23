package concurrent

import (
	"sync"
	"testing"
)

func TestSliceAppend(t *testing.T) {
	s := &Slice{}

	s.Append("item1")
	s.Append("item2")
	s.Append(42)

	// Verify items through iteration
	items := []interface{}{}
	for item := range s.Iter() {
		items = append(items, item.Value)
	}

	if len(items) != 3 {
		t.Errorf("Slice has %d items, want 3", len(items))
	}
}

func TestSliceIter(t *testing.T) {
	s := &Slice{}

	expected := []interface{}{"a", "b", "c"}
	for _, v := range expected {
		s.Append(v)
	}

	// Collect items with their indices
	found := make(map[int]interface{})
	for item := range s.Iter() {
		found[item.Index] = item.Value
	}

	if len(found) != len(expected) {
		t.Errorf("Iter() returned %d items, want %d", len(found), len(expected))
	}

	for i, expectedVal := range expected {
		if foundVal, ok := found[i]; !ok || foundVal != expectedVal {
			t.Errorf("Iter() at index %d: got %v, want %v", i, foundVal, expectedVal)
		}
	}
}

func TestSliceDelete(t *testing.T) {
	s := &Slice{}

	s.Append("a")
	s.Append("b")
	s.Append("c")

	// Delete middle item
	s.Delete("b")

	// Verify remaining items
	items := []interface{}{}
	for item := range s.Iter() {
		items = append(items, item.Value)
	}

	if len(items) != 2 {
		t.Errorf("After delete, slice has %d items, want 2", len(items))
	}

	// Verify "b" is not in the slice
	for _, item := range items {
		if item == "b" {
			t.Error("Deleted item 'b' still found in slice")
		}
	}
}

func TestSliceDeleteNonexistent(t *testing.T) {
	s := &Slice{}

	s.Append("a")
	s.Append("b")

	// Delete item that doesn't exist
	s.Delete("nonexistent")

	// Verify items remain unchanged
	items := []interface{}{}
	for item := range s.Iter() {
		items = append(items, item.Value)
	}

	if len(items) != 2 {
		t.Errorf("After delete nonexistent, slice has %d items, want 2", len(items))
	}
}

func TestSliceDeleteFirst(t *testing.T) {
	s := &Slice{}

	s.Append("a")
	s.Append("b")
	s.Append("c")

	// Delete first item
	s.Delete("a")

	items := []interface{}{}
	for item := range s.Iter() {
		items = append(items, item.Value)
	}

	if len(items) != 2 {
		t.Errorf("After delete first, slice has %d items, want 2", len(items))
	}
}

func TestSliceDeleteLast(t *testing.T) {
	s := &Slice{}

	s.Append("a")
	s.Append("b")
	s.Append("c")

	// Delete last item
	s.Delete("c")

	items := []interface{}{}
	for item := range s.Iter() {
		items = append(items, item.Value)
	}

	if len(items) != 2 {
		t.Errorf("After delete last, slice has %d items, want 2", len(items))
	}
}

func TestSliceIterEmpty(t *testing.T) {
	s := &Slice{}

	count := 0
	for range s.Iter() {
		count++
	}

	if count != 0 {
		t.Errorf("Iter() on empty slice returned %d items, want 0", count)
	}
}

func TestSliceConcurrentAccess(t *testing.T) {
	s := &Slice{}

	var wg sync.WaitGroup

	// Concurrent appends
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Append(n)
		}(i)
	}

	wg.Wait()

	// Count items
	count := 0
	for range s.Iter() {
		count++
	}

	if count != 100 {
		t.Errorf("After concurrent appends, slice has %d items, want 100", count)
	}
}

func TestSliceConcurrentDeleteAndAppend(t *testing.T) {
	s := &Slice{}

	// Pre-populate
	for i := 0; i < 10; i++ {
		s.Append(i)
	}

	var wg sync.WaitGroup

	// Concurrent operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				s.Append(n + 100)
			} else {
				s.Delete(n % 10)
			}
		}(i)
	}

	wg.Wait()

	// If we get here without panicking or deadlocking, the concurrent access worked
}

func TestSliceDeleteAll(t *testing.T) {
	s := &Slice{}

	items := []interface{}{"a", "b", "c"}
	for _, item := range items {
		s.Append(item)
	}

	// Delete all items
	for _, item := range items {
		s.Delete(item)
	}

	// Verify slice is empty
	count := 0
	for range s.Iter() {
		count++
	}

	if count != 0 {
		t.Errorf("After deleting all items, slice has %d items, want 0", count)
	}
}
