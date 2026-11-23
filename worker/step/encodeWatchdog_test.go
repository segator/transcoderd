package step

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEncodeWatchdog(t *testing.T) {
	t.Run("stalled process is cancelled", func(t *testing.T) {
		started, done := make(chan struct{}), make(chan struct{})
		close(started)
		cancelled := false
		if !watchEncodeProgress(context.Background(), started, done, 40*time.Millisecond,
			func() (int64, int64, bool) { return 1, 1, false },
			func() { cancelled = true }) || !cancelled {
			t.Fatal("stalled process was not cancelled")
		}
	})
	t.Run("pause and continuing progress prevent cancellation", func(t *testing.T) {
		started, done := make(chan struct{}), make(chan struct{})
		close(started)
		var frame atomic.Int64
		var paused atomic.Bool
		paused.Store(true)
		result := make(chan bool, 1)
		go func() {
			result <- watchEncodeProgress(context.Background(), started, done, 80*time.Millisecond,
				func() (int64, int64, bool) { return frame.Load(), 0, paused.Load() },
				func() { t.Error("cancelled a paused or progressing process") })
		}()
		time.Sleep(120 * time.Millisecond)
		paused.Store(false)
		for i := 0; i < 8; i++ {
			frame.Add(1)
			time.Sleep(15 * time.Millisecond)
		}
		close(done)
		if <-result {
			t.Fatal("watchdog reported a stall")
		}
	})
	t.Run("not started is not timed", func(t *testing.T) {
		started, done := make(chan struct{}), make(chan struct{})
		result := make(chan bool, 1)
		go func() {
			result <- watchEncodeProgress(context.Background(), started, done, 20*time.Millisecond,
				func() (int64, int64, bool) { return 0, 0, false },
				func() { t.Error("cancelled an unstarted process") })
		}()
		time.Sleep(50 * time.Millisecond)
		close(done)
		if <-result {
			t.Fatal("watchdog reported a stall")
		}
	})
}

func TestStoreMaximum(t *testing.T) {
	var value atomic.Int64
	for _, n := range []int64{10, 5, 10, 11} {
		storeMaximum(&value, n)
	}
	if got := value.Load(); got != 11 {
		t.Fatalf("maximum = %d, want 11", got)
	}
}
