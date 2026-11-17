package worker

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

// ConfigureProtectedMode must finish before Run and transfers workspace-lock
// ownership to the executor. Unsupported servers or changed workspaces fail
// closed; there is no implicit fallback to legacy assignments.
func (e *JobExecutor) ConfigureProtectedMode(ctx context.Context, client *serverclient.ProtectedClient, workspace *job.WorkspaceLock, identity uuid.UUID) error {
	if client == nil || workspace == nil {
		return errors.New("protected client and persistent workspace are required")
	}
	path, err := workspace.Path()
	if err != nil {
		return err
	}
	expected, err := filepath.Abs(e.tempPath)
	if err != nil || filepath.Clean(path) != filepath.Clean(expected) {
		return errors.New("protected workspace does not match worker workspace")
	}
	if err := workspace.VerifyPersistentIdentity(identity); err != nil {
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = client.Negotiate(checkCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("negotiate protected protocol: %w", err)
	}
	_, pending, err := workspace.ExistingPendingRequest()
	if err != nil {
		return err
	}
	if err := e.configureProtectedClient(client); err != nil {
		return err
	}
	e.admissionMu.Lock()
	e.protectedWorkspace = workspace
	e.protectedWorkspaceID = identity
	e.protectedReconciled = false
	e.safeIdleEnabled = false
	e.protectedRecoveryPending = pending
	if e.encodePool != nil {
		e.encodePool.SetReconciliationBarrier(true)
	}
	if pending {
		e.reservedJobs++
	}
	e.admissionMu.Unlock()
	return nil
}

func (e *JobExecutor) reconcileProtectedInventory(ctx context.Context) error {
	assignments, err := e.protectedClient.Assignments(ctx)
	if err != nil {
		return err
	}
	e.protectedMu.Lock()
	defer e.protectedMu.Unlock()
	seen := make(map[uuid.UUID]bool, len(assignments))
	for _, assignment := range assignments {
		execution := e.protectedJobs[assignment.JobID]
		if execution == nil {
			return fmt.Errorf("worker has an unreconciled assignment %s", assignment.JobID)
		}
		execution.mu.Lock()
		valid := (assignment.State == "assigned" || assignment.State == "running" || assignment.State == "parked" || assignment.State == "publishing") && assignment.Generation != 0 && execution.job != nil && execution.job.Assignment != nil && execution.job.Assignment.Generation == assignment.Generation
		execution.mu.Unlock()
		if !valid {
			return fmt.Errorf("worker has an unreconciled assignment %s", assignment.JobID)
		}
		seen[assignment.JobID] = true
	}
	for id := range e.protectedJobs {
		if !seen[id] {
			return fmt.Errorf("local protected assignment %s is no longer owned by this worker", id)
		}
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.protectedRecoveryPending || e.reservedJobs != 0 || e.recoveryError != "" {
		return errors.New("worker has unresolved protected recovery")
	}
	e.protectedReconciled = true
	e.safeIdleEnabled = true
	if e.encodePool != nil {
		e.encodePool.SetReconciliationBarrier(false)
	}
	return nil
}
