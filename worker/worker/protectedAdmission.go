package worker

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type protectedRequestClient interface {
	RequestJob(context.Context, uuid.UUID) (*model.ProtectedAssignment, error)
}

// requestProtectedJob resolves a persisted request before admitting a new one.
// The assignment is durably checkpointed before the request journal is marked
// resolved, so a lost response or crash cannot silently consume another job.
func requestProtectedJob(ctx context.Context, workspace *job.WorkspaceLock, client protectedRequestClient, prepare func(*job.Context) error) (*job.Context, error) {
	if workspace == nil || client == nil || prepare == nil {
		return nil, errors.New("protected admission is not configured")
	}
	root, err := workspace.Path()
	if err != nil {
		return nil, err
	}
	requestID, err := workspace.PendingRequest()
	if err != nil {
		return nil, err
	}
	assigned, err := client.RequestJob(ctx, requestID)
	if errors.Is(err, serverclient.ErrNoJobAvailable) {
		if err := workspace.ResolveRequest(requestID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if assigned == nil || assigned.Id == uuid.Nil || assigned.EventID < 0 || assigned.Generation == 0 || assigned.Capability == "" || assigned.KeyID == "" {
		return nil, errors.New("server returned an invalid protected assignment")
	}
	jobDir := filepath.Join(root, assigned.Id.String())
	contextFile := filepath.Join(jobDir, assigned.Id.String()+".json")
	if info, statErr := os.Lstat(jobDir); statErr == nil && !info.IsDir() {
		return nil, errors.New("protected job workspace is not a directory")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	jobContext, err := job.ReadContextFromDiskByPath(contextFile)
	switch {
	case errors.Is(err, os.ErrNotExist):
		jobContext = job.NewContext(assigned.Id, assigned.EventID, jobDir)
		jobContext.Source.OriginalName = assigned.Name
		jobContext.RequestID = requestID
		jobContext.Assignment = &model.AssignmentClaim{JobID: assigned.Id, Generation: assigned.Generation, Capability: assigned.Capability}
		if err := jobContext.Init(); err != nil {
			return nil, fmt.Errorf("create protected job workspace: %w", err)
		}
		if err := prepare(jobContext); err != nil {
			return nil, fmt.Errorf("prepare protected job: %w", err)
		}
		if err := jobContext.PersistJobContext(); err != nil {
			return nil, fmt.Errorf("persist protected job assignment: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("recover protected job assignment: %w", err)
	default:
		claim := jobContext.Assignment
		if jobContext.JobId != assigned.Id || filepath.Clean(jobContext.WorkingDir) != filepath.Clean(jobDir) || jobContext.RequestID != requestID || jobContext.EventId != assigned.EventID || claim == nil || claim.JobID != assigned.Id || claim.Generation != assigned.Generation || subtle.ConstantTimeCompare([]byte(claim.Capability), []byte(assigned.Capability)) != 1 {
			return nil, errors.New("persisted protected job does not match the request result")
		}
	}
	if err := workspace.ResolveRequest(requestID); err != nil {
		return nil, fmt.Errorf("resolve protected job request: %w", err)
	}
	return jobContext, nil
}
