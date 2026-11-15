package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"

	"transcoder/model"
)

type protectedProgressKey struct {
	jobID, progressID string
	generation        uint64
	phase             model.NotificationType
}

type protectedProgressState struct {
	sequence uint64
	hash     [sha256.Size]byte
}

func (r *InMemoryRepository) ApplyProtectedProgress(ctx context.Context, worker string, claim model.AssignmentClaim, progress model.TaskProgressType, sequence uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	hash, err := protectedProgressHash(worker, claim, progress, sequence)
	if err != nil {
		return err
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	id := claim.JobID.String()
	state, ok := r.protectedJobs[id]
	provided := sha256.Sum256([]byte(claim.Capability))
	last := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
	if !ok || state.worker != worker || state.generation != claim.Generation || (state.state != "assigned" && state.state != "running") || subtle.ConstantTimeCompare(state.capabilityHash[:], provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus || !memoryProtectedLeaseValid(state, claim, false) {
		return ErrStaleAssignment
	}
	key := protectedProgressKey{id, progress.ProgressID, claim.Generation, progress.NotificationType}
	if previous, exists := r.protectedProgress[key]; exists {
		if previous.sequence == sequence && subtle.ConstantTimeCompare(previous.hash[:], hash[:]) == 1 {
			return nil
		}
		if previous.sequence >= sequence {
			return ErrEventConflict
		}
	}
	for _, existing := range r.progress {
		if existing.ProgressID == progress.ProgressID && existing.NotificationType == progress.NotificationType && (existing.JobId != claim.JobID || existing.WorkerName != worker) {
			return ErrEventConflict
		}
	}
	progressKey := fmt.Sprintf("protected:%s:%s", progress.ProgressID, progress.NotificationType)
	if progress.Status == model.ProgressingTaskProgressTypeStatus {
		value := progress
		r.progress[progressKey] = &value
	} else {
		delete(r.progress, progressKey)
	}
	r.protectedProgress[key] = protectedProgressState{sequence: sequence, hash: hash}
	return nil
}
