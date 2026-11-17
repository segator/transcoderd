package worker

import (
	"context"
	"errors"
	"time"
)

var ErrSafeIdleUnavailable = errors.New("safe-idle capability is unavailable")

// BeginRestartBarrier atomically blocks new reservations and encode starts.
// Running work and independent RAM/job holds are left untouched.
func (e *JobExecutor) BeginRestartBarrier() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.restartBarrier = true
	if e.encodePool != nil {
		e.encodePool.SetRestartBarrier(true)
	}
}

// ReleaseRestartBarrier restores the current queue and pause settings; it does
// not change persisted limits or resume suspended processes.
func (e *JobExecutor) ReleaseRestartBarrier() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.restartBarrier = false
	if e.encodePool != nil {
		e.encodePool.SetRestartBarrier(false)
	}
}

// WaitSafeIdle holds the barrier even on cancellation or timeout; the caller
// must explicitly release it if a restart will not follow.
func (e *JobExecutor) WaitSafeIdle(ctx context.Context) error {
	e.BeginRestartBarrier()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := e.AdmissionStatus()
		if status.SafeIdle == nil {
			return ErrSafeIdleUnavailable
		}
		if *status.SafeIdle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
