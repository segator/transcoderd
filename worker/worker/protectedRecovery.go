package worker

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type protectedRecoveryClient interface {
	protectedEventClient
	protectedParkClient
	AcquireLease(context.Context, model.AssignmentClaim) (time.Time, error)
}

// A terminal event can be replayed without a live lease after the server has
// recorded the exact completion publication or failure. Otherwise it must
// first grant a new process lease before accepting the event.
func recoverableTerminalAssignment(j *job.Context) bool {
	if j == nil || j.Assignment == nil || j.Assignment.JobID != j.JobId || j.Assignment.Generation == 0 || j.Assignment.Capability == "" || j.Assignment.InstanceID == uuid.Nil || j.RequestID == uuid.Nil || j.Parked || j.ParkIntent {
		return false
	}
	if pending := j.PendingProtectedEvent(); pending != nil {
		return pending.NotificationType == model.JobNotification && (pending.Status == model.CompletedNotificationStatus || pending.Status == model.FailedNotificationStatus)
	}
	return j.LastEvent != nil && j.LastEvent.NotificationType == model.JobNotification && (j.LastEvent.Status == model.CompletedNotificationStatus || j.LastEvent.Status == model.FailedNotificationStatus)
}

func recoverProtectedTerminal(ctx context.Context, j *job.Context, worker string, client protectedRecoveryClient) error {
	if !recoverableTerminalAssignment(j) || worker == "" || client == nil {
		return errors.New("protected terminal checkpoint cannot be reconciled")
	}
	publisher := &protectedEventPublisher{client: client, workerName: worker, retryDelay: time.Second}
	if j.PendingProtectedEvent() != nil {
		err := publisher.Reconcile(ctx, j)
		if errors.Is(err, serverclient.ErrProtectedConflict) {
			claim := *j.Assignment
			claim.InstanceID = uuid.New()
			guard, leaseErr := startLeaseGuard(ctx, func(ctx context.Context) (time.Time, error) {
				return client.AcquireLease(ctx, claim)
			}, 5*time.Second)
			if leaseErr != nil {
				return leaseErr
			}
			defer guard.Close()
			if err := j.RecordAcquiredLease(claim.InstanceID); err != nil {
				return err
			}
			err = publisher.Reconcile(guard.ctx, j)
		}
		if err != nil {
			return err
		}
	}
	return j.Clean()
}

// Only checkpoints before Download starts are replayed here. Other interrupted
// phases require their own recovery policy and continue to fail closed.
func recoverableInitialAssignment(j *job.Context) bool {
	if j == nil || j.Assignment == nil || j.Assignment.JobID != j.JobId || j.Assignment.Generation == 0 || j.Assignment.Capability == "" || j.RequestID == uuid.Nil || j.Parked || j.ParkIntent || (j.NextPhase != "" && j.NextPhase != model.DownloadNotification) {
		return false
	}
	if j.LastEvent != nil && (j.LastEvent.NotificationType != model.JobNotification || j.LastEvent.Status != model.StartedNotificationStatus) {
		return false
	}
	if pending := j.PendingProtectedEvent(); pending != nil && (pending.NotificationType != model.JobNotification || pending.Status != model.StartedNotificationStatus || j.LastEvent != nil) {
		return false
	}
	return true
}

func recoverablePreEncodeCheckpoint(j *job.Context) bool {
	if j == nil || j.Assignment == nil || j.Assignment.JobID != j.JobId || j.Assignment.Generation == 0 || j.Assignment.Capability == "" {
		return false
	}
	phase, status := model.NotificationType(""), model.NotificationStatus("")
	if pending := j.PendingProtectedEvent(); pending != nil {
		// A phase completion can be persisted before its response is received.
		// Only replay it when the preceding event proves that phase had started.
		if j.LastEvent == nil || j.LastEvent.NotificationType != pending.NotificationType || j.LastEvent.Status != model.StartedNotificationStatus || pending.JobId != j.JobId {
			return false
		}
		phase, status = pending.NotificationType, pending.Status
	} else if j.LastEvent != nil {
		phase, status = j.LastEvent.NotificationType, j.LastEvent.Status
	}
	if status != model.CompletedNotificationStatus {
		return false
	}
	switch phase {
	case model.DownloadNotification, model.MKVExtractNotification, model.PGSNotification:
	default:
		return false
	}
	if j.NextPhase == model.FFMPEGSNotification {
		return j.PendingProtectedEvent() == nil
	}
	// The completion event was persisted before OnComplete saved the next
	// phase. Infer FFmpeg only when the same routing rules prove that no
	// subtitle preparation remains. Never infer it from a parked snapshot.
	if j.NextPhase != phase || j.Parked || j.ParkIntent || j.Source == nil || j.Source.FFProbeData == nil {
		return false
	}
	for _, subtitle := range j.Source.FFProbeData.Subtitle {
		if subtitle == nil {
			return false
		}
	}
	switch phase {
	case model.DownloadNotification:
		return !j.Source.FFProbeData.HaveExtractableSubtitle()
	case model.MKVExtractNotification:
		return !j.Source.FFProbeData.HaveImageTypeSubtitle()
	default:
		return true
	}
}

// recoverParkedJob reconciles only a validated pre-encode checkpoint. The
// caller must serialize the queue-stop decision with resume and admission.
// Running jobs and other unresolved contexts need their own recovery path.
func recoverParkedJob(ctx context.Context, j *job.Context, worker string, client protectedRecoveryClient, queueStopped bool, retryDelay time.Duration) (*leaseGuard, error) {
	if !recoverablePreEncodeCheckpoint(j) || client == nil || worker == "" {
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
	if j.NextPhase != model.FFMPEGSNotification {
		previous := j.NextPhase
		j.NextPhase = model.FFMPEGSNotification
		if err := j.PersistJobContext(); err != nil {
			j.NextPhase = previous
			return nil, fmt.Errorf("persist recovered pre-encode boundary: %w", err)
		}
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

// restoreProtectedParked installs the per-job encode hold before queueing a
// recovered checkpoint. The scanner resumes it only after settings permit it.
func (e *JobExecutor) restoreProtectedParked(ctx context.Context, j *job.Context) error {
	if e.protectedClient == nil || e.encodePool == nil || !recoverablePreEncodeCheckpoint(j) {
		return errors.New("protected parked recovery is unavailable")
	}
	if err := e.encodePool.holdEncoder(j.JobId); err != nil {
		return err
	}
	if _, err := recoverParkedJob(ctx, j, e.workerConfig.Name, e.protectedClient, true, time.Second); err != nil {
		return err
	}
	executor := e.stepExecutors[model.FFMPEGSNotification]
	if executor == nil {
		return errors.New("protected encode queue is unavailable")
	}
	execution := &protectedExecution{job: j, events: &protectedEventPublisher{client: e.protectedClient, workerName: e.workerConfig.Name}}
	e.protectedMu.Lock()
	if e.protectedJobs == nil || e.protectedJobs[j.JobId] != nil {
		e.protectedMu.Unlock()
		return errors.New("protected recovered job is already tracked")
	}
	e.protectedJobs[j.JobId] = execution
	e.protectedMu.Unlock()
	e.admissionMu.Lock()
	atomic.AddUint32(&e.activeJobs, 1)
	e.jobs.set(j, model.FFMPEGSNotification, "reconciling")
	e.admissionMu.Unlock()
	executor.AddJob(j)
	e.admissionMu.Lock()
	e.jobs.set(j, model.FFMPEGSNotification, "parked")
	e.admissionMu.Unlock()
	return nil
}
