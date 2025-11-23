package concurrent

import (
	"sync"
	"testing"
)

func TestMapSetAndGet(t *testing.T) {
	m := &Map{items: make(map[string]interface{})}

	m.Set("key1", "value1")
	m.Set("key2", 42)
	m.Set("key3", true)

	// Test Get for existing keys
	val1, ok1 := m.Get("key1")
	if !ok1 || val1 != "value1" {
		t.Errorf("Get(key1) = %v, %v, want value1, true", val1, ok1)
	}

	val2, ok2 := m.Get("key2")
	if !ok2 || val2 != 42 {
		t.Errorf("Get(key2) = %v, %v, want 42, true", val2, ok2)
	}

	val3, ok3 := m.Get("key3")
	if !ok3 || val3 != true {
		t.Errorf("Get(key3) = %v, %v, want true, true", val3, ok3)
	}

	// Test Get for non-existing key
	val4, ok4 := m.Get("nonexistent")
	if ok4 {
		t.Errorf("Get(nonexistent) = %v, %v, want nil, false", val4, ok4)
	}
}

func TestMapOverwrite(t *testing.T) {
	m := &Map{items: make(map[string]interface{})}

	m.Set("key", "value1")
	val1, _ := m.Get("key")
	if val1 != "value1" {
		t.Errorf("First set failed: got %v, want value1", val1)
	}

	m.Set("key", "value2")
	val2, _ := m.Get("key")
	if val2 != "value2" {
		t.Errorf("Overwrite failed: got %v, want value2", val2)
	}
}

func TestMapIter(t *testing.T) {
	m := &Map{items: make(map[string]interface{})}

	expected := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}

	for k, v := range expected {
		m.Set(k, v)
	}

	// Collect items from iterator
	found := make(map[string]interface{})
	for item := range m.Iter() {
		found[item.Key] = item.Value
	}

	// Verify all items were found
	if len(found) != len(expected) {
		t.Errorf("Iter() returned %d items, want %d", len(found), len(expected))
	}

	for k, v := range expected {
		if foundVal, ok := found[k]; !ok || foundVal != v {
			t.Errorf("Iter() missing or wrong value for key %s: got %v, want %v", k, foundVal, v)
		}
	}
}

func TestMapConcurrentAccess(t *testing.T) {
	m := &Map{items: make(map[string]interface{})}

	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Set(string(rune('a'+n%26)), n)
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Get(string(rune('a' + n%26)))
		}(i)
	}

	wg.Wait()

	// If we get here without panicking, the concurrent access worked
}

func TestMapIterEmpty(t *testing.T) {
	m := &Map{items: make(map[string]interface{})}

	count := 0
	for range m.Iter() {
		count++
	}

	if count != 0 {
		t.Errorf("Iter() on empty map returned %d items, want 0", count)
	}
}
