package worker

import (
	"context"
	"errors"
	"time"

	"transcoder/model"
	"transcoder/worker/job"

	"github.com/google/uuid"
)

type protectedRecoveryClient interface {
	protectedEventClient
	protectedParkClient
	AcquireLease(context.Context, model.AssignmentClaim) (time.Time, error)
}

// recoverParkedJob reconciles only a validated pre-encode checkpoint. The
// caller must serialize the queue-stop decision with resume and admission.
// Running jobs and other unresolved contexts need their own recovery path.
func recoverParkedJob(ctx context.Context, j *job.Context, worker string, client protectedRecoveryClient, queueStopped bool, retryDelay time.Duration) (*leaseGuard, error) {
	if j == nil || j.Assignment == nil || j.Assignment.JobID != j.JobId || (!j.Parked && !j.ParkIntent) || j.NextPhase != model.FFMPEGSNotification || client == nil || worker == "" {
		return nil, errors.New("protected parked checkpoint cannot be reconciled")
	}
	if err := j.ValidatePrefetchedSource(); err != nil {
		return nil, err
	}
	instance := uuid.New()
	claim := *j.Assignment
	claim.InstanceID = instance
	guard, err := startLeaseGuard(ctx, func(ctx context.Context) (time.Time, error) {
		return client.AcquireLease(ctx, claim)
	}, 5*time.Second)
	if err != nil {
		return nil, err
	}
	guard.instance = instance
	keepGuard := false
	defer func() {
		if !keepGuard {
			guard.Close()
		}
	}()
	if err := j.RecordAcquiredLease(instance); err != nil {
		return nil, err
	}
	publisher := &protectedEventPublisher{client: client, workerName: worker, retryDelay: retryDelay}
	if err := publisher.Reconcile(guard.ctx, j); err != nil {
		return nil, err
	}
	if queueStopped {
		if err := parkProtectedJob(ctx, j, guard, client, retryDelay); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := j.ResumeWithLease(instance); err != nil {
		return nil, err
	}
	if err := guard.ctx.Err(); err != nil {
		return nil, ErrProtectedLeaseLost
	}
	keepGuard = true
	return guard, nil
}
