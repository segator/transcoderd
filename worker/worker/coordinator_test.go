package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForActiveJobs(t *testing.T) {
	var active atomic.Uint32
	active.Store(2)

	go func() {
		time.Sleep(10 * time.Millisecond)
		active.Store(0)
	}()

	if !waitForActiveJobs(context.Background(), active.Load, time.Millisecond) {
		t.Fatal("waitForActiveJobs() = false, want true after active jobs finish")
	}
}

func TestWaitForActiveJobsStopsWhenContextIsCanceled(t *testing.T) {
	var active atomic.Uint32
	active.Store(1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	if waitForActiveJobs(ctx, active.Load, time.Millisecond) {
		t.Fatal("waitForActiveJobs() = true, want false after cancellation")
	}
}
