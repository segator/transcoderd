package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/job"

	"github.com/google/uuid"
)

func TestRestartBarrierBlocksAdmissionAndEncodeStarts(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 2}, maxActiveJobs: 2, encodePool: newEncodeConcurrency(2)}
	e.BeginRestartBarrier()
	if e.ReserveJobSlot() || e.AdmissionStatus().Accepting || !e.AdmissionStatus().RestartBarrier {
		t.Fatal("restart barrier did not block admission")
	}
	if _, ok := e.encodePool.tryStartEncoder(""); ok {
		t.Fatal("restart barrier allowed an encode to start")
	}
	// A concurrent queue resume may change desired limits, but must not open
	// the barrier before the restart supervisor releases it.
	if err := e.SetEncoderLimits(1, 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.encodePool.tryStartEncoder(""); ok {
		t.Fatal("changing encoder limits released the restart barrier")
	}
	e.ReleaseRestartBarrier()
	if !e.ReserveJobSlot() {
		t.Fatal("admission did not reopen after releasing the barrier")
	}
	e.ReleaseJobSlot()
	kind, ok := e.encodePool.tryStartEncoder("")
	if !ok {
		t.Fatal("encoder did not reopen after releasing the barrier")
	}
	e.encodePool.finishEncoder(kind)
}

func TestWaitSafeIdleRequiresConfirmedParkedJobsAndHoldsBarrier(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 1}, maxActiveJobs: 1, encodePool: newEncodeConcurrency(1), safeIdleEnabled: true}
	j := &job.Context{JobId: uuid.New()}
	atomic.StoreUint32(&e.activeJobs, 1)
	e.jobs.set(j, model.FFMPEGSNotification, "parked")
	if err := e.WaitSafeIdle(t.Context()); err != nil {
		t.Fatal(err)
	}
	status := e.AdmissionStatus()
	if status.SafeIdle == nil || !*status.SafeIdle || status.ParkedJobs != 1 || !status.RestartBarrier || e.ReserveJobSlot() {
		t.Fatalf("safe idle status = %+v", status)
	}
	if err := e.SetMaxActiveJobs(2); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.encodePool.tryStartEncoder("", j.JobId); ok {
		t.Fatal("queue resume bypassed the successful idle barrier")
	}
	e.ReleaseRestartBarrier()
	if status := e.AdmissionStatus(); status.SafeIdle == nil || *status.SafeIdle {
		t.Fatalf("idle after barrier release = %+v", status)
	}
}

func TestWaitSafeIdleFailsClosedOnUnknownAndTimeout(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 1}, maxActiveJobs: 1, encodePool: newEncodeConcurrency(1)}
	if err := e.WaitSafeIdle(t.Context()); !errors.Is(err, ErrSafeIdleUnavailable) || !e.AdmissionStatus().RestartBarrier {
		t.Fatalf("unknown safe idle = %v, status = %+v", err, e.AdmissionStatus())
	}
	e.ReleaseRestartBarrier()
	e.safeIdleEnabled = true
	atomic.StoreUint32(&e.activeJobs, 1) // No confirmed parked record.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := e.WaitSafeIdle(ctx); !errors.Is(err, context.DeadlineExceeded) || !e.AdmissionStatus().RestartBarrier {
		t.Fatalf("timed-out safe idle = %v, status = %+v", err, e.AdmissionStatus())
	}
}

func TestSafeIdleCountsEncodeAlreadyStarting(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 1}, maxActiveJobs: 1, encodePool: newEncodeConcurrency(1), safeIdleEnabled: true}
	j := &job.Context{JobId: uuid.New()}
	atomic.StoreUint32(&e.activeJobs, 1)
	e.jobs.set(j, model.FFMPEGSNotification, "parked")
	kind, ok := e.encodePool.tryStartEncoder("", j.JobId)
	if !ok {
		t.Fatal("test encode did not acquire its slot")
	}
	e.BeginRestartBarrier()
	if status := e.AdmissionStatus(); status.SafeIdle == nil || *status.SafeIdle {
		t.Fatalf("encode in transition was reported idle: %+v", status)
	}
	e.encodePool.finishEncoder(kind, j.JobId)
	if status := e.AdmissionStatus(); status.SafeIdle == nil || !*status.SafeIdle {
		t.Fatalf("parked job after encode finishes = %+v", status)
	}
}

func TestEncodeHoldSerializesParkingWithEncodeStart(t *testing.T) {
	for i := 0; i < 100; i++ {
		pool := newEncodeConcurrency(1)
		id := uuid.New()
		start := make(chan struct{})
		var wg sync.WaitGroup
		var held bool
		var kind string
		var running bool
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			held = pool.holdEncoder(id) == nil
		}()
		go func() {
			defer wg.Done()
			<-start
			kind, running = pool.tryStartEncoder("", id)
		}()
		close(start)
		wg.Wait()
		if held == running {
			t.Fatalf("hold=%t encode started=%t, want exactly one", held, running)
		}
		if running {
			pool.finishEncoder(kind, id)
		} else {
			pool.SetPaused(false)
			if _, ok := pool.tryStartEncoder("", id); ok {
				t.Fatal("queue resume bypassed a pending park hold")
			}
			pool.releaseEncoder(id)
			kind, ok := pool.tryStartEncoder("", id)
			if !ok {
				t.Fatal("encode did not start after hold release")
			}
			pool.finishEncoder(kind, id)
		}
	}
}

func TestRestartBarrierPreservesStoppedQueue(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 0}, maxActiveJobs: 0, encodePool: newEncodeConcurrency(1)}
	e.encodePool.SetPaused(true)
	e.BeginRestartBarrier()
	e.ReleaseRestartBarrier()
	if e.ReserveJobSlot() || !e.encodePool.Paused() {
		t.Fatal("releasing restart barrier changed the stopped queue")
	}
}
