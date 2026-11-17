package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type protectedExecution struct {
	mu     sync.Mutex
	job    *job.Context
	guard  *leaseGuard
	events *protectedEventPublisher
}

func (p *protectedExecution) currentGuard() *leaseGuard {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.guard
}

var ErrProtectedEventUncertain = errors.New("protected event requires reconciliation")

// configureProtectedClient is used before starting the worker. Protected jobs
// cannot use legacy media transfer or event routes.
func (e *JobExecutor) configureProtectedClient(client *serverclient.ProtectedClient) error {
	if client == nil || e.client == nil {
		return errors.New("protected worker client is unavailable")
	}
	e.protectedMu.Lock()
	defer e.protectedMu.Unlock()
	if e.protectedStarted || e.ActiveJobs() != 0 {
		return errors.New("protected worker must be configured before startup")
	}
	e.protectedClient = client
	e.protectedReconciled = true // Test/internal setup; full mode reconciles before enabling admission.
	e.protectedJobs = make(map[uuid.UUID]*protectedExecution)
	executors := setupStepExecutors(e)
	e.trackPipeline(executors)
	e.stepExecutors = executors
	return nil
}

func (e *JobExecutor) protectedExecution(id uuid.UUID) *protectedExecution {
	e.protectedMu.Lock()
	defer e.protectedMu.Unlock()
	return e.protectedJobs[id]
}

func (e *JobExecutor) startProtectedReservedJob(ctx context.Context, j *job.Context) error {
	if j == nil || j.Assignment == nil || e.protectedClient == nil {
		return errors.New("protected assignment is unavailable")
	}
	if e.protectedWorkspace != nil {
		if err := e.protectedWorkspace.VerifyPersistentIdentity(e.protectedWorkspaceID); err != nil {
			return fmt.Errorf("persistent workspace identity is unavailable: %w", err)
		}
	}
	e.admissionMu.Lock()
	hasReservation := e.reservedJobs > 0
	e.admissionMu.Unlock()
	if !hasReservation {
		return errors.New("protected job has no reserved admission slot")
	}
	claim := *j.Assignment
	claim.InstanceID = uuid.New()
	guard, err := startLeaseGuard(ctx, func(ctx context.Context) (time.Time, error) {
		return e.protectedClient.AcquireLease(ctx, claim)
	}, 5*time.Second)
	if err != nil {
		return err
	}
	guard.instance = claim.InstanceID
	keepGuard := false
	defer func() {
		if !keepGuard {
			guard.Close()
		}
	}()
	if err := j.RecordAcquiredLease(claim.InstanceID); err != nil {
		return err
	}
	publisher := &protectedEventPublisher{client: e.protectedClient, workerName: e.workerConfig.Name}
	if j.LastEvent == nil || j.LastEvent.NotificationType != model.JobNotification || j.LastEvent.Status != model.StartedNotificationStatus {
		if err := publisher.Publish(guard.ctx, j, model.JobNotification, model.StartedNotificationStatus, ""); err != nil {
			return err
		}
	}
	if err := guard.ctx.Err(); err != nil {
		return ErrProtectedLeaseLost
	}
	e.protectedMu.Lock()
	if e.protectedJobs == nil || e.protectedJobs[j.JobId] != nil {
		e.protectedMu.Unlock()
		return errors.New("protected job is already tracked or not configured")
	}
	e.admissionMu.Lock()
	if e.reservedJobs <= 0 {
		e.admissionMu.Unlock()
		e.protectedMu.Unlock()
		return errors.New("protected job has no reserved admission slot")
	}
	e.reservedJobs--
	atomic.AddUint32(&e.activeJobs, 1)
	e.jobs.set(j, model.DownloadNotification, "queued")
	e.protectedJobs[j.JobId] = &protectedExecution{job: j, guard: guard, events: publisher}
	e.admissionMu.Unlock()
	e.protectedMu.Unlock()
	keepGuard = true
	e.stepExecutors[model.DownloadNotification].AddJob(j)
	return nil
}

func (e *JobExecutor) publishStepEvent(ctx context.Context, j *job.Context, phase model.NotificationType, status model.NotificationStatus, message string) error {
	if j.Assignment == nil {
		e.publishTaskEvent(j, phase, status, message)
		return nil
	}
	execution := e.protectedExecution(j.JobId)
	if execution == nil {
		return ErrProtectedLeaseLost
	}
	guard := execution.currentGuard()
	if guard == nil || guard.ctx.Err() != nil {
		return ErrProtectedLeaseLost
	}
	if err := execution.events.Publish(ctx, j, phase, status, message); err != nil {
		return fmt.Errorf("%w: %w", ErrProtectedEventUncertain, err)
	}
	return nil
}

func (e *JobExecutor) finishProtectedSuccess(j *job.Context) {
	execution := e.protectedExecution(j.JobId)
	if execution == nil {
		e.blockProtectedJob(j, ErrProtectedLeaseLost)
		return
	}
	guard := execution.currentGuard()
	if guard == nil || guard.ctx.Err() != nil {
		e.blockProtectedJob(j, ErrProtectedLeaseLost)
		return
	}
	if err := execution.events.Publish(guard.ctx, j, model.JobNotification, model.CompletedNotificationStatus, ""); err != nil {
		// Publication may have completed while the response or lease renewal
		// was lost. The exact durable event can be retried independently: the
		// server accepts it without a lease only after committing that intent.
		pending := j.PendingProtectedEvent()
		if pending == nil || pending.NotificationType != model.JobNotification || pending.Status != model.CompletedNotificationStatus || e.serviceCtx == nil || e.serviceCtx.Err() != nil {
			e.blockProtectedJob(j, err)
			return
		}
		ctx, cancel := context.WithTimeout(e.serviceCtx, 10*time.Second)
		reconcileErr := execution.events.Reconcile(ctx, j)
		cancel()
		if reconcileErr != nil {
			e.blockProtectedJob(j, errors.Join(err, reconcileErr))
			return
		}
	}
	e.finishProtectedJob(j, execution)
}

func isDefinitiveProtectedFailure(cause error) bool {
	return cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) && !errors.Is(cause, ErrProtectedLeaseLost) && !errors.Is(cause, ErrProtectedEventUncertain) && !errors.Is(cause, serverclient.ErrProtectedConflict) && !errors.Is(cause, serverclient.ErrProtectedUnavailable)
}

func (e *JobExecutor) finishProtectedFailure(j *job.Context, phase model.NotificationType, cause error) {
	execution := e.protectedExecution(j.JobId)
	if execution == nil {
		e.blockProtectedJob(j, cause)
		return
	}
	guard := execution.currentGuard()
	if guard == nil || guard.ctx.Err() != nil || !isDefinitiveProtectedFailure(cause) {
		e.blockProtectedJob(j, cause)
		return
	}
	if err := execution.events.Publish(guard.ctx, j, model.JobNotification, model.FailedNotificationStatus, fmt.Sprintf("%s:%s", phase, cause)); err != nil {
		e.blockProtectedJob(j, err)
		return
	}
	e.finishProtectedJob(j, execution)
}

func (e *JobExecutor) finishProtectedJob(j *job.Context, execution *protectedExecution) {
	e.protectedMu.Lock()
	delete(e.protectedJobs, j.JobId)
	e.protectedMu.Unlock()
	if guard := execution.currentGuard(); guard != nil {
		guard.Close()
	}
	if err := j.Clean(); err != nil {
		e.recordRecoveryError(fmt.Errorf("clean protected job %s: %w", j.JobId, err), false)
	}
	// Keep the job counted until the lease and workspace cleanup are finished.
	_ = e.cleanTrackedJob(func() error { return nil }, j.JobId.String())
}

func (e *JobExecutor) blockProtectedJob(j *job.Context, cause error) {
	if cause == nil {
		cause = ErrProtectedLeaseLost
	}
	if execution := e.protectedExecution(j.JobId); execution != nil {
		if guard := execution.currentGuard(); guard != nil {
			guard.Close()
		}
	}
	e.admissionMu.Lock()
	e.jobs.set(j, model.JobNotification, "blocked")
	e.admissionMu.Unlock()
	e.recordRecoveryError(fmt.Errorf("protected job %s needs reconciliation: %w", j.JobId, cause), false)
}
