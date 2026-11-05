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
func (r *RuntimeScheduler) ControlWorker(ctx context.Context, name, action string) (*model.WorkerControl, error) {
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
	c := model.WorkerControl{ID: uuid.NewString(), Action: action, State: "pending", CPUJobs: restore.CPUJobs, GPUJobs: restore.GPUJobs, MaxActiveJobs: restore.MaxActiveJobs}
	if err := r.repo.SetWorkerControl(ctx, name, c); err != nil {
		return nil, err
	}
	return &c, nil
}
