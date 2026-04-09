package max

import (
	"testing"
)

// Mock implementation of Extractor for testing
type mockExtractor struct {
	data []int
}

func (m *mockExtractor) Len() int {
	return len(m.data)
}

func (m *mockExtractor) Less(i, j int) bool {
	return m.data[i] < m.data[j]
}

func (m *mockExtractor) Swap(i, j int) {
	m.data[i], m.data[j] = m.data[j], m.data[i]
}

func (m *mockExtractor) GetLastElement(i int) interface{} {
	return m.data[i]
}

func TestMax(t *testing.T) {
	tests := []struct {
		name     string
		data     []int
		expected int
	}{
		{
			name:     "Simple case",
			data:     []int{1, 5, 3, 2, 4},
			expected: 5,
		},
		{
			name:     "Already sorted",
			data:     []int{1, 2, 3, 4, 5},
			expected: 5,
		},
		{
			name:     "Reverse sorted",
			data:     []int{5, 4, 3, 2, 1},
			expected: 5,
		},
		{
			name:     "Single element",
			data:     []int{42},
			expected: 42,
		},
		{
			name:     "Two elements",
			data:     []int{10, 20},
			expected: 20,
		},
		{
			name:     "Negative numbers",
			data:     []int{-5, -2, -8, -1},
			expected: -1,
		},
		{
			name:     "Mixed positive and negative",
			data:     []int{-5, 3, -2, 8, 0},
			expected: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &mockExtractor{data: tt.data}
			result := Max(extractor)

			if result.(int) != tt.expected {
				t.Errorf("Max() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMaxWithDuplicates(t *testing.T) {
	extractor := &mockExtractor{data: []int{3, 5, 5, 2, 5, 1}}
	result := Max(extractor)

	if result.(int) != 5 {
		t.Errorf("Max() with duplicates = %v, want 5", result)
	}
}

func TestMaxSortsData(t *testing.T) {
	extractor := &mockExtractor{data: []int{3, 1, 4, 1, 5}}
	originalLen := len(extractor.data)

	_ = Max(extractor)

	// Verify data is sorted after Max()
	if extractor.data[0] != 1 {
		t.Error("Data should be sorted after Max()")
	}

	if extractor.data[originalLen-1] != 5 {
		t.Errorf("Last element after sort = %d, want 5", extractor.data[originalLen-1])
	}

	// Verify all data is sorted
	for i := 0; i < len(extractor.data)-1; i++ {
		if extractor.data[i] > extractor.data[i+1] {
			t.Errorf("Data not properly sorted at index %d", i)
		}
	}
}
