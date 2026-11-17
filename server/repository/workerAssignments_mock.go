package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"transcoder/model"
)

func (r *InMemoryRepository) ListWorkerAssignments(ctx context.Context, worker string) ([]model.WorkerAssignmentSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if worker == "" {
		return nil, errors.New("worker identity is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	assignments := make([]model.WorkerAssignmentSnapshot, 0)
	for id, job := range r.jobs {
		last := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
		if last == nil || last.WorkerName != worker || (last.Status != model.AssignedNotificationStatus && last.Status != model.StartedNotificationStatus) {
			continue
		}
		item := model.WorkerAssignmentSnapshot{JobID: job.Id, Status: string(last.Status), State: "legacy"}
		if state, ok := r.protectedJobs[id]; ok {
			item.Generation, item.State = state.generation, state.state
			if state.worker != worker {
				item.State = "inconsistent"
			}
		}
		assignments = append(assignments, item)
		if len(assignments) > maxWorkerAssignmentInventory {
			return nil, fmt.Errorf("worker assignment inventory exceeds %d entries", maxWorkerAssignmentInventory)
		}
	}
	sort.Slice(assignments, func(i, j int) bool { return assignments[i].JobID.String() < assignments[j].JobID.String() })
	return assignments, nil
}
