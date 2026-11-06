package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

var ErrWorkerControlUnavailable = errors.New("worker control unavailable")

func (r *RuntimeScheduler) GetWorkerControl(ctx context.Context, name string) (*model.WorkerControl, error) {
	return r.repo.GetWorkerControl(ctx, name)
}
func (r *RuntimeScheduler) AckWorkerControl(ctx context.Context, name string, ack model.WorkerControlAck) error {
	return r.repo.AckWorkerControl(ctx, name, ack)
}
func (r *RuntimeScheduler) ControlWorker(ctx context.Context, name, action string, jobIDs ...string) (*model.WorkerControl, error) {
	if err := model.ValidateWorkerAction(action); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkerSettings, err)
	}
	worker, err := r.repo.GetWorker(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%w: worker not available", ErrWorkerControlUnavailable)
	}
	rt := worker.Runtime
	if time.Since(worker.LastSeen) > 30*time.Second || rt == nil || rt.ControlVersion < 1 || rt.Draining || (rt.Finishing && action != "resume-queue") {
		return nil, fmt.Errorf("%w: worker must be online, updated and outside maintenance", ErrWorkerControlUnavailable)
	}
	jobID := ""
	if len(jobIDs) > 1 {
		return nil, fmt.Errorf("%w: only one job can be controlled", ErrInvalidWorkerSettings)
	}
	if len(jobIDs) == 1 {
		jobID = jobIDs[0]
	}
	individual := action == "pause-job" || action == "resume-job"
	if individual {
		id, err := uuid.Parse(jobID)
		if err != nil {
			return nil, fmt.Errorf("%w: a valid jobId is required", ErrInvalidWorkerSettings)
		}
		jobID = id.String()
		if rt.ControlVersion < 2 {
			return nil, fmt.Errorf("%w: update this worker for individual encode controls", ErrWorkerControlUnavailable)
		}
		owned := false
		for _, j := range rt.Jobs {
			if j.ID == jobID && j.Phase == string(model.FFMPEGSNotification) && j.CanPause && (j.State == "running" || j.State == "paused") {
				owned = true
				break
			}
		}
		if !owned {
			return nil, fmt.Errorf("%w: encode is no longer active on this worker", ErrWorkerControlUnavailable)
		}
	} else if jobID != "" {
		return nil, fmt.Errorf("%w: jobId is only valid for individual controls", ErrInvalidWorkerSettings)
	}
	restore, err := r.repo.GetWorkerResumeSettings(ctx, name)
	if err != nil {
		return nil, err
	}
	if restore.MaxActiveJobs == nil && !restore.Independent() {
		previous, err := r.repo.GetWorkerControl(ctx, name)
		if err != nil {
			return nil, err
		}
		if previous != nil {
			snapshot := previous.Settings()
			restore = &snapshot
		}
	}
	if restore.Independent() {
		restore.MaxActiveJobs = nil
	}
	if restore.MaxActiveJobs == nil && !restore.Independent() && rt.CPUJobs+rt.GPUJobs > 0 {
		cpu, gpu := rt.CPUJobs, rt.GPUJobs
		restore = &model.WorkerSettings{CPUJobs: &cpu, GPUJobs: &gpu}
	}
	if action == "resume-queue" {
		if err := restore.ValidateWrite(); err != nil {
			return nil, fmt.Errorf("%w: save positive limits before resuming the queue", ErrWorkerControlUnavailable)
		}
	}
	c := model.WorkerControl{ID: uuid.NewString(), JobID: jobID, Action: action, State: "pending", CPUJobs: restore.CPUJobs, GPUJobs: restore.GPUJobs, MaxActiveJobs: restore.MaxActiveJobs}
	if err := r.repo.SetWorkerControl(ctx, name, c); err != nil {
		return nil, err
	}
	return &c, nil
}
