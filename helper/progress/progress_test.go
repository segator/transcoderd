package progress

import (
	"io"
	"testing"
	"time"
)

func TestProgressN(t *testing.T) {
	p := Progress{n: 1024}
	if p.N() != 1024 {
		t.Errorf("N() = %d, want 1024", p.N())
	}
}

func TestProgressSize(t *testing.T) {
	p := Progress{size: 2048}
	if p.Size() != 2048 {
		t.Errorf("Size() = %d, want 2048", p.Size())
	}
}

func TestProgressSpeed(t *testing.T) {
	p := Progress{speed: 1024.5}
	if p.Speed() != 1024.5 {
		t.Errorf("Speed() = %f, want 1024.5", p.Speed())
	}
}

func TestProgressErr(t *testing.T) {
	err := io.EOF
	p := Progress{err: err}
	if p.Err() != io.EOF {
		t.Errorf("Err() = %v, want %v", p.Err(), io.EOF)
	}
}

func TestProgressComplete(t *testing.T) {
	tests := []struct {
		name string
		p    Progress
		want bool
	}{
		{
			name: "EOF means complete",
			p:    Progress{n: 50, size: 100, err: io.EOF},
			want: true,
		},
		{
			name: "n >= size means complete",
			p:    Progress{n: 100, size: 100},
			want: true,
		},
		{
			name: "n > size means complete",
			p:    Progress{n: 150, size: 100},
			want: true,
		},
		{
			name: "Unknown size (-1) never complete",
			p:    Progress{n: 1000, size: -1},
			want: false,
		},
		{
			name: "Not yet complete",
			p:    Progress{n: 50, size: 100},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Complete()
			if got != tt.want {
				t.Errorf("Complete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProgressPercent(t *testing.T) {
	tests := []struct {
		name string
		p    Progress
		want float64
	}{
		{
			name: "Zero progress",
			p:    Progress{n: 0, size: 100},
			want: 0,
		},
		{
			name: "Half complete",
			p:    Progress{n: 50, size: 100},
			want: 50,
		},
		{
			name: "Fully complete",
			p:    Progress{n: 100, size: 100},
			want: 100,
		},
		{
			name: "Over 100%",
			p:    Progress{n: 150, size: 100},
			want: 100,
		},
		{
			name: "25% complete",
			p:    Progress{n: 25, size: 100},
			want: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Percent()
			if got != tt.want {
				t.Errorf("Percent() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestProgressRemaining(t *testing.T) {
	future := time.Now().Add(time.Hour)

	tests := []struct {
		name     string
		p        Progress
		wantZero bool
		wantNeg  bool
	}{
		{
			name:    "No estimate",
			p:       Progress{estimated: time.Time{}},
			wantNeg: true,
		},
		{
			name:     "With future estimate",
			p:        Progress{estimated: future},
			wantZero: false,
			wantNeg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Remaining()
			if tt.wantNeg && got != -1 {
				t.Errorf("Remaining() = %v, want -1", got)
			}
			if tt.wantZero && got != 0 {
				t.Errorf("Remaining() = %v, want 0", got)
			}
		})
	}
}

func TestProgressEstimated(t *testing.T) {
	now := time.Now()
	p := Progress{estimated: now}

	if !p.Estimated().Equal(now) {
		t.Errorf("Estimated() = %v, want %v", p.Estimated(), now)
	}

	// Test zero time
	p2 := Progress{}
	if !p2.Estimated().IsZero() {
		t.Error("Estimated() should be zero time by default")
	}
}

// Mock Counter for testing
type mockCounter struct {
	n   int64
	err error
}

func (m *mockCounter) N() int64 {
	return m.n
}

func (m *mockCounter) Err() error {
	return m.err
}

func TestCounterInterface(t *testing.T) {
	counter := &mockCounter{n: 1024, err: nil}

	if counter.N() != 1024 {
		t.Errorf("Counter.N() = %d, want 1024", counter.N())
	}

	if counter.Err() != nil {
		t.Errorf("Counter.Err() = %v, want nil", counter.Err())
	}

	counter.err = io.EOF
	if counter.Err() != io.EOF {
		t.Errorf("Counter.Err() = %v, want EOF", counter.Err())
	}
}
