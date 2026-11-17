package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type protectedParkClient interface {
	Park(context.Context, model.AssignmentClaim) error
}

// parkProtectedJob persists the boundary and stops lease renewal before asking
// the server to park. Otherwise a concurrent renewal could resume the server
// assignment after the park was acknowledged. ctx is the service context,
// independent of the lease guard's execution context.
func parkProtectedJob(ctx context.Context, j *job.Context, guard *leaseGuard, client protectedParkClient, retryDelay time.Duration) error {
	if j == nil || j.Assignment == nil || guard == nil || client == nil {
		return errors.New("protected parking is not configured")
	}
	if err := j.PreparePark(); err != nil {
		return err
	}
	if guard.ctx.Err() != nil {
		return ErrProtectedLeaseLost
	}
	guard.Close()
	if errors.Is(context.Cause(guard.ctx), ErrProtectedLeaseLost) {
		return ErrProtectedLeaseLost
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := client.Park(ctx, *j.Assignment)
		if err == nil {
			if err := j.ConfirmPark(); err != nil {
				return fmt.Errorf("record protected park acknowledgement: %w", err)
			}
			return nil
		}
		if errors.Is(err, serverclient.ErrProtectedConflict) || errors.Is(err, serverclient.ErrProtectedUnavailable) {
			return err
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// parkPrefetched holds the individual encode before contacting the server, so
// a concurrent queue resume cannot start it during park reconciliation.
func (e *JobExecutor) parkPrefetched(ctx context.Context, j *job.Context, guard *leaseGuard, client protectedParkClient, retryDelay time.Duration) error {
	if j == nil || e.encodePool == nil {
		return errors.New("prefetched encode is unavailable")
	}
	e.admissionMu.Lock()
	err := e.encodePool.holdEncoder(j.JobId)
	e.admissionMu.Unlock()
	if err != nil {
		return err
	}
	if err := parkProtectedJob(ctx, j, guard, client, retryDelay); err != nil {
		return err // Keep the hold until the uncertain assignment is reconciled.
	}
	e.admissionMu.Lock()
	e.jobs.set(j, model.FFMPEGSNotification, "parked")
	e.admissionMu.Unlock()
	return nil
}

// releaseParkedEncode is called only after ownership is reclaimed and the
// recovered context has been durably updated with the new lease.
func (e *JobExecutor) releaseParkedEncode(j *job.Context, guard *leaseGuard) error {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if j == nil || guard == nil || guard.ctx.Err() != nil || !j.ReadyForProtectedEncode(guard.instance) || e.encodePool == nil {
		return errors.New("protected parked encode is not ready")
	}
	if e.protectedWorkspace != nil && e.protectedWorkspace.VerifyPersistentIdentity(e.protectedWorkspaceID) != nil {
		return errors.New("persistent workspace identity is unavailable")
	}
	if e.restartBarrier || e.admissionHeld || e.finishing || e.settingsPending || e.recoveryError != "" || e.ramPaused || e.maxActiveJobs <= 0 {
		return errors.New("worker queue is not ready to resume parked work")
	}
	if e.reclaimedJobs == nil {
		e.reclaimedJobs = make(map[uuid.UUID]bool)
	}
	e.reclaimedJobs[j.JobId] = true
	e.jobs.set(j, model.FFMPEGSNotification, "queued")
	e.encodePool.releaseEncoder(j.JobId)
	return nil
}
