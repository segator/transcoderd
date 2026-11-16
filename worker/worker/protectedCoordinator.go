package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	"transcoder/update"
	"transcoder/worker/job"
)

func (q *ServerCoordinator) protectedRequestTaskRoutine(ctx context.Context) {
	q.worker.admissionMu.Lock()
	pending := q.worker.protectedRecoveryPending
	q.worker.admissionMu.Unlock()
	if pending && !q.resolveProtectedReservation(ctx) {
		return
	}
	for _, j := range q.worker.initialRecoveries {
		if !q.resolveInitialProtectedAssignment(ctx, j) {
			return
		}
	}
	q.worker.initialRecoveries = nil
	for ctx.Err() == nil {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := q.worker.reconcileProtectedInventory(checkCtx)
		cancel()
		q.worker.admissionMu.Lock()
		if err != nil {
			q.worker.protectedInventoryError = fmt.Sprintf("protected ownership reconciliation: %v", err)
		} else {
			q.worker.protectedInventoryError = ""
		}
		q.worker.admissionMu.Unlock()
		if err == nil {
			break
		}
		if !waitProtectedRetry(ctx, 5*time.Second) {
			return
		}
	}
	for ctx.Err() == nil {
		if !waitProtectedRetry(ctx, 5*time.Second) {
			return
		}
		if err := q.syncWorkerSettings(ctx); err != nil {
			q.logger.Errorf("Error synchronizing protected worker capacity: %v", err)
			continue
		}
		if !q.worker.ReserveJobSlot() {
			continue
		}
		if q.updater != nil {
			_, updateRequired, err := q.updater.CheckForUpdate()
			if err != nil {
				q.worker.ReleaseJobSlot()
				q.logger.Errorf("Error checking for update: %v", err)
				continue
			}
			if updateRequired {
				q.worker.ReleaseJobSlot()
				q.worker.BeginRestartBarrier()
				q.logger.Logf("Worker update is waiting for protected safe idle")
				if err := q.worker.WaitSafeIdle(ctx); err == nil {
					os.Exit(update.ExitCode)
				}
				return
			}
		}
		if !q.resolveProtectedReservation(ctx) {
			return
		}
	}
}

func (q *ServerCoordinator) resolveInitialProtectedAssignment(ctx context.Context, j *job.Context) bool {
	if !q.worker.beginProtectedOperation() {
		return false
	}
	defer q.worker.protectedOps.Done()
	for ctx.Err() == nil {
		if err := q.worker.startProtectedReservedJob(ctx, j); err == nil {
			return true
		} else {
			q.logger.Errorf("Protected initial assignment remains unresolved: %v", err)
		}
		if !waitProtectedRetry(ctx, 5*time.Second) {
			return false
		}
	}
	return false
}

// resolveProtectedReservation keeps the slot while a request or its returned
// assignment is uncertain. It retries the same durable identity and context.
func (q *ServerCoordinator) resolveProtectedReservation(ctx context.Context) bool {
	if !q.worker.beginProtectedOperation() {
		return false
	}
	defer q.worker.protectedOps.Done()
	var pendingJob *job.Context
	for ctx.Err() == nil {
		if pendingJob == nil {
			requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			j, err := requestProtectedJob(requestCtx, q.worker.protectedWorkspace, q.worker.protectedClient, q.worker.pinAdmissionProfile)
			cancel()
			if err != nil {
				q.logger.Errorf("Protected job request remains unresolved: %v", err)
				if !waitProtectedRetry(ctx, 5*time.Second) {
					return false
				}
				continue
			}
			if j == nil || q.worker.protectedExecution(j.JobId) != nil {
				q.worker.ReleaseJobSlot()
				q.clearProtectedRecoveryPending()
				return true
			}
			pendingJob = j
		}
		if err := q.worker.startProtectedReservedJob(ctx, pendingJob); err != nil {
			q.logger.Errorf("Protected assignment remains unresolved: %v", err)
			if !waitProtectedRetry(ctx, 5*time.Second) {
				return false
			}
			continue
		}
		q.clearProtectedRecoveryPending()
		return true
	}
	return false
}

func (q *ServerCoordinator) clearProtectedRecoveryPending() {
	q.worker.admissionMu.Lock()
	q.worker.protectedRecoveryPending = false
	q.worker.admissionMu.Unlock()
}

func waitProtectedRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
