package worker

import (
	"context"
	"errors"
	"time"

	"transcoder/model"
)

func (e *JobExecutor) protectedParkingRoutine(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.reconcileProtectedQueue(ctx)
		}
	}
}

func (e *JobExecutor) reconcileProtectedQueue(ctx context.Context) {
	// Startup inventory owns reconciliation until the server and local
	// assignments agree. The independent encode barrier keeps queued work
	// from starting while this check is in progress.
	e.admissionMu.Lock()
	reconciled := e.protectedReconciled
	e.admissionMu.Unlock()
	if !reconciled {
		return
	}
	e.protectedMu.Lock()
	jobs := make([]*protectedExecution, 0, len(e.protectedJobs))
	for _, execution := range e.protectedJobs {
		jobs = append(jobs, execution)
	}
	e.protectedMu.Unlock()
	for _, execution := range jobs {
		if ctx.Err() != nil {
			return
		}
		e.reconcileProtectedJob(ctx, execution)
	}
}

func (e *JobExecutor) protectedQueueStoppedLocked() bool {
	return e.restartBarrier || e.maxActiveJobs <= 0 || e.ramPaused || e.settingsPending || e.recoveryError != "" || (e.protectedClient != nil && !e.protectedReconciled) || e.admissionHeld || e.finishing
}

func (e *JobExecutor) reconcileProtectedJob(ctx context.Context, execution *protectedExecution) {
	execution.mu.Lock()
	j := execution.job
	if j == nil {
		execution.mu.Unlock()
		return
	}
	e.admissionMu.Lock()
	status, exists := e.jobs.get(j.JobId.String())
	stopped := e.protectedQueueStoppedLocked()
	if exists && status.State == "parked" && status.Phase == string(model.FFMPEGSNotification) && !stopped {
		// Invalidate the idle proof before the server changes ownership.
		e.jobs.set(j, model.FFMPEGSNotification, "reconciling")
	}
	e.admissionMu.Unlock()
	if !exists || status.Phase != string(model.FFMPEGSNotification) {
		execution.mu.Unlock()
		return
	}
	if status.State == "queued" && stopped {
		guard := execution.guard
		err := e.parkPrefetched(ctx, j, guard, e.protectedClient, time.Second)
		execution.mu.Unlock()
		if err != nil && !errors.Is(err, ErrEncodeAlreadyStarted) && ctx.Err() == nil {
			e.blockProtectedJob(j, err)
		}
		return
	}
	if status.State != "parked" || stopped {
		execution.mu.Unlock()
		return
	}
	guard, err := recoverParkedJob(ctx, j, e.workerConfig.Name, e.protectedClient, false, time.Second)
	if err == nil {
		execution.guard = guard
		err = e.releaseParkedEncode(j, guard)
		if err != nil {
			// A concurrent stop or restart barrier won the race. The per-job
			// hold still prevents FFmpeg from starting; park it again.
			err = e.parkPrefetched(ctx, j, guard, e.protectedClient, time.Second)
		}
	}
	execution.mu.Unlock()
	if err != nil && ctx.Err() == nil {
		e.blockProtectedJob(j, err)
	}
}
