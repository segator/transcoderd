package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"transcoder/model"
	"transcoder/server/assignment"

	"github.com/google/uuid"
)

type protectedRequestKey struct{ worker, request string }
type protectedRequestResult struct {
	jobID, keyID, name string
	generation         uint64
	eventID            int
}
type protectedJobState struct {
	worker, keyID, state string
	generation           uint64
	capabilityHash       [sha256.Size]byte
}
type protectedUploadKey struct {
	jobID      string
	generation uint64
}
type protectedUpload struct {
	path, checksum, state, message, finalPath string
	size                                      int64
	eventID                                   int
	deleteSource                              bool
	cleanupPending                            bool
}

func (r *InMemoryRepository) BeginProtectedPublication(ctx context.Context, worker string, claim model.AssignmentClaim, event model.TaskEventType, deleteSource bool) (*model.ProtectedPublication, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || event.JobId != claim.JobID || event.WorkerName != worker || event.NotificationType != model.JobNotification || event.Status != model.CompletedNotificationStatus || event.EventID < 0 {
		return nil, ErrStaleAssignment
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	id := claim.JobID.String()
	state, ok := r.protectedJobs[id]
	provided := sha256.Sum256([]byte(claim.Capability))
	if !ok || state.worker != worker || state.generation != claim.Generation || subtle.ConstantTimeCompare(state.capabilityHash[:], provided[:]) != 1 || state.state == "revoked" || state.state == "parked" {
		return nil, ErrStaleAssignment
	}
	key := protectedUploadKey{id, claim.Generation}
	upload, ok := r.protectedUploads[key]
	if !ok {
		return nil, ErrEventConflict
	}
	if state.state == "publishing" || state.state == "completed" {
		if upload.state != state.state || upload.eventID != event.EventID || upload.message != event.Message {
			return nil, ErrEventConflict
		}
	} else {
		lastJob := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
		lastEvent := model.TaskEvents(r.events[id]).GetLatest()
		if (state.state != "assigned" && state.state != "running") || upload.state != "staged" || lastJob == nil || lastJob.WorkerName != worker || lastJob.Status != model.StartedNotificationStatus || lastEvent == nil || lastEvent.EventID+1 != event.EventID {
			return nil, ErrEventConflict
		}
		state.state = "publishing"
		r.protectedJobs[id] = state
		upload.state, upload.eventID, upload.message, upload.deleteSource = "publishing", event.EventID, event.Message, deleteSource
		r.protectedUploads[key] = upload
	}
	return &model.ProtectedPublication{JobID: claim.JobID, Generation: claim.Generation, WorkerName: worker, StagingPath: upload.path, Checksum: upload.checksum, SizeBytes: upload.size, EventID: upload.eventID, Message: upload.message, State: upload.state, FinalPath: upload.finalPath, DeleteSource: upload.deleteSource, CleanupPending: upload.cleanupPending}, nil
}

func (r *InMemoryRepository) FinishProtectedPublication(ctx context.Context, publication model.ProtectedPublication, finalPath string, size int64, probe *model.MediaProbe) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if publication.JobID == uuid.Nil || publication.Generation == 0 || !filepath.IsLocal(finalPath) || size < 0 || probe == nil {
		return errors.New("invalid protected publication result")
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	id := publication.JobID.String()
	state, ok := r.protectedJobs[id]
	job := r.jobs[id]
	if !ok || job == nil || state.generation != publication.Generation {
		return ErrStaleAssignment
	}
	key := protectedUploadKey{id, publication.Generation}
	upload, ok := r.protectedUploads[key]
	if !ok {
		return ErrStaleAssignment
	}
	if upload.eventID != publication.EventID || upload.message != publication.Message || state.worker != publication.WorkerName || upload.path != publication.StagingPath || upload.checksum != publication.Checksum || upload.size != publication.SizeBytes {
		return ErrEventConflict
	}
	if state.state == "completed" && upload.state == "completed" {
		if upload.finalPath != finalPath {
			return ErrEventConflict
		}
		return nil
	}
	if state.state != "publishing" || upload.state != "publishing" {
		return ErrStaleAssignment
	}
	last := model.TaskEvents(r.events[id]).GetLatest()
	if last == nil || last.EventID+1 != upload.eventID {
		return ErrEventConflict
	}
	job.TargetPath, job.TargetSize, job.TargetProbe = finalPath, size, probe
	event := &model.TaskEventType{Event: model.Event{WorkerName: state.worker, EventTime: time.Now().UTC()}, JobId: publication.JobID, EventID: upload.eventID, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus, Message: upload.message}
	r.events[id] = append(r.events[id], event)
	job.Status, job.StatusPhase, job.StatusMessage, job.LastUpdate = string(event.Status), event.NotificationType, event.Message, &event.EventTime
	state.state, upload.state, upload.finalPath = "completed", "completed", finalPath
	r.protectedJobs[id], r.protectedUploads[key] = state, upload
	return nil
}

func (r *InMemoryRepository) ListPendingProtectedPublications(ctx context.Context) ([]model.ProtectedPublication, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []model.ProtectedPublication
	for key, upload := range r.protectedUploads {
		state := r.protectedJobs[key.jobID]
		if state.generation != key.generation || (upload.state != "publishing" && !((upload.state == "completed" || upload.state == "abandoned") && (upload.deleteSource || upload.cleanupPending))) {
			continue
		}
		id, err := uuid.Parse(key.jobID)
		if err != nil {
			return nil, err
		}
		result = append(result, model.ProtectedPublication{JobID: id, Generation: key.generation, WorkerName: state.worker, StagingPath: upload.path, Checksum: upload.checksum, SizeBytes: upload.size, EventID: upload.eventID, Message: upload.message, State: upload.state, FinalPath: upload.finalPath, DeleteSource: upload.deleteSource, CleanupPending: upload.cleanupPending})
	}
	return result, nil
}

func (r *InMemoryRepository) ClearProtectedSourceDeletion(ctx context.Context, id string, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	key := protectedUploadKey{id, generation}
	upload, ok := r.protectedUploads[key]
	if ok && (upload.state == "completed" || upload.state == "abandoned") {
		upload.deleteSource = false
		upload.cleanupPending = false
		r.protectedUploads[key] = upload
	}
	return nil
}

func (r *InMemoryRepository) RequestProtectedJob(ctx context.Context, worker, requestID string, keys *assignment.Keyring) (*model.ProtectedAssignment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(worker) == "" || strings.TrimSpace(worker) != worker || len(worker) > 100 || keys == nil {
		return nil, errors.New("invalid protected job request")
	}
	requestUUID, err := uuid.Parse(requestID)
	if err != nil {
		return nil, fmt.Errorf("invalid request identity: %w", err)
	}
	request := protectedRequestKey{worker, requestUUID.String()}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if prior, exists := r.protectedRequests[request]; exists {
		return protectedResponse(keys, worker, prior)
	}
	var chosen *model.Job
	var chosenEvent *model.TaskEventType
	for id, candidate := range r.jobs {
		events := model.TaskEvents(r.events[id])
		status := events.GetLatestPerNotificationType(model.JobNotification)
		if status == nil || status.Status != model.QueuedNotificationStatus {
			continue
		}
		state := r.protectedJobs[id]
		if state.generation > 0 && state.state != "revoked" && state.state != "completed" {
			continue
		}
		blocked := false
		for key, upload := range r.protectedUploads {
			if key.jobID == id && (upload.state == "completed" || upload.state == "abandoned") && (upload.deleteSource || upload.cleanupPending) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		if chosen == nil || status.EventTime.Before(chosenEvent.EventTime) || (status.EventTime.Equal(chosenEvent.EventTime) && id < chosen.Id.String()) {
			chosen, chosenEvent = candidate, status
		}
	}
	if chosen == nil {
		r.protectedRequests[request] = protectedRequestResult{}
		return nil, nil
	}
	id := chosen.Id.String()
	state := r.protectedJobs[id]
	if state.generation > 0 && state.state != "revoked" && state.state != "completed" {
		return nil, errors.New("protected assignment is not revoked")
	}
	for key, upload := range r.protectedUploads {
		if key.jobID == id && (upload.state == "completed" || upload.state == "abandoned") && (upload.deleteSource || upload.cleanupPending) {
			return nil, ErrJobPublishing
		}
	}
	state.generation++
	state.worker, state.keyID, state.state = worker, keys.ActiveKeyID(), "assigned"
	capability, err := keys.Capability(state.keyID, worker, id, state.generation)
	if err != nil {
		return nil, err
	}
	state.capabilityHash = sha256.Sum256([]byte(capability))
	events := model.TaskEvents(r.events[id])
	job := *chosen
	job.Events = append(model.TaskEvents(nil), events...)
	event := job.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
	event.WorkerName, event.EventTime = worker, time.Now().UTC()
	r.events[id] = append(r.events[id], event)
	chosen.Status, chosen.StatusPhase, chosen.StatusMessage = string(event.Status), event.NotificationType, event.Message
	chosen.LastUpdate = &event.EventTime
	r.protectedJobs[id] = state
	prior := protectedRequestResult{jobID: id, keyID: state.keyID, name: filepath.Base(chosen.SourcePath), generation: state.generation, eventID: event.EventID}
	r.protectedRequests[request] = prior
	return protectedResponse(keys, worker, prior)
}

func protectedResponse(keys *assignment.Keyring, worker string, request protectedRequestResult) (*model.ProtectedAssignment, error) {
	if request.jobID == "" {
		return nil, nil
	}
	id, err := uuid.Parse(request.jobID)
	if err != nil {
		return nil, err
	}
	capability, err := keys.Capability(request.keyID, worker, request.jobID, request.generation)
	if err != nil {
		return nil, err
	}
	return &model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: id, EventID: request.eventID, Name: request.name}, Generation: request.generation, KeyID: request.keyID, Capability: capability}, nil
}

func (r *InMemoryRepository) GetJobForMutation(ctx context.Context, id string) (*model.Job, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[id]
	if !ok {
		return nil, "", ErrElementNotFound
	}
	copyJob := *job
	copyJob.Events = append(model.TaskEvents(nil), r.events[id]...)
	for key, upload := range r.protectedUploads {
		if key.jobID == id && (upload.state == "completed" || upload.state == "abandoned") && (upload.deleteSource || upload.cleanupPending) {
			return nil, "", ErrJobPublishing
		}
	}
	return &copyJob, r.protectedJobs[id].state, nil
}

func (r *InMemoryRepository) IsProtectedJob(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.protectedJobs[id]
	return ok, nil
}

func (r *InMemoryRepository) WithLegacyJob(ctx context.Context, id string, fn func(Repository, *model.Job) error) error {
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	job, err := r.GetJob(ctx, id)
	if err != nil {
		return err
	}
	protected, err := r.IsProtectedJob(ctx, id)
	if err != nil {
		return err
	}
	if protected {
		return ErrStaleAssignment
	}
	return fn(r, job)
}

func (r *InMemoryRepository) ApplyProtectedEvent(ctx context.Context, worker string, claim model.AssignmentClaim, event model.TaskEventType) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateProtectedEvent(worker, claim, event); err != nil {
		return err
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	id := claim.JobID.String()
	state, ok := r.protectedJobs[id]
	job := r.jobs[id]
	provided := sha256.Sum256([]byte(claim.Capability))
	if !ok || job == nil || state.worker != worker || state.generation != claim.Generation || state.state == "revoked" || subtle.ConstantTimeCompare(state.capabilityHash[:], provided[:]) != 1 {
		return ErrStaleAssignment
	}
	for _, existing := range r.events[id] {
		if existing.EventID == event.EventID {
			if sameProtectedEvent(existing, event) {
				return nil
			}
			return ErrEventConflict
		}
	}
	if state.state != "assigned" && state.state != "running" {
		return ErrStaleAssignment
	}
	latest := model.TaskEvents(r.events[id]).GetLatest()
	if latest == nil || event.EventID != latest.EventID+1 {
		return ErrEventConflict
	}
	event.EventTime = time.Now().UTC()
	r.events[id] = append(r.events[id], &event)
	job.Status, job.StatusPhase, job.StatusMessage, job.LastUpdate = string(event.Status), event.NotificationType, event.Message, &event.EventTime
	if event.NotificationType == model.JobNotification && event.Status == model.FailedNotificationStatus {
		state.state = "completed"
	} else if event.NotificationType == model.FFMPEGSNotification && event.Status == model.StartedNotificationStatus {
		state.state = "running"
	}
	r.protectedJobs[id] = state
	return nil
}

func (r *InMemoryRepository) StageProtectedUpload(ctx context.Context, worker string, claim model.AssignmentClaim, path, checksum string, size int64, stage func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateUploadMetadata(path, checksum, size, stage); err != nil {
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
	if !ok || worker == "" || claim.Capability == "" || state.worker != worker || state.generation != claim.Generation || (state.state != "assigned" && state.state != "running") || subtle.ConstantTimeCompare(state.capabilityHash[:], provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
		return ErrStaleAssignment
	}
	key := protectedUploadKey{id, claim.Generation}
	existing, exists := r.protectedUploads[key]
	if exists && (existing.path != path || existing.checksum != checksum || existing.size != size || existing.state != "staged") {
		return ErrEventConflict
	}
	if err := stage(); err != nil {
		return fmt.Errorf("stage protected upload: %w", err)
	}
	r.protectedUploads[key] = protectedUpload{path: path, checksum: checksum, size: size, state: "staged", cleanupPending: true}
	return nil
}

func (r *InMemoryRepository) ParkProtectedJob(ctx context.Context, worker string, claim model.AssignmentClaim) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	id := claim.JobID.String()
	state, ok := r.protectedJobs[id]
	last := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
	provided := sha256.Sum256([]byte(claim.Capability))
	if !ok || claim.Capability == "" || state.worker != worker || state.generation != claim.Generation || (state.state != "assigned" && state.state != "parked") || subtle.ConstantTimeCompare(state.capabilityHash[:], provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
		return ErrStaleAssignment
	}
	state.state = "parked"
	r.protectedJobs[id] = state
	return nil
}

func (r *InMemoryRepository) GetProtectedJob(ctx context.Context, worker string, claim model.AssignmentClaim) (*model.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := claim.JobID.String()
	state, ok := r.protectedJobs[id]
	job := r.jobs[id]
	last := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
	provided := sha256.Sum256([]byte(claim.Capability))
	if !ok || job == nil || worker == "" || claim.Capability == "" || state.worker != worker || state.generation != claim.Generation || (state.state != "assigned" && state.state != "running") || subtle.ConstantTimeCompare(state.capabilityHash[:], provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
		return nil, ErrStaleAssignment
	}
	copyJob := *job
	copyJob.Events = append(model.TaskEvents(nil), r.events[id]...)
	return &copyJob, nil
}

func (r *InMemoryRepository) CancelJobAssignment(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return ErrElementNotFound
	}
	last := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
	if last == nil || (last.Status != model.AssignedNotificationStatus && last.Status != model.StartedNotificationStatus) {
		return fmt.Errorf("job %s cannot be canceled in its current state", id)
	}
	state, protected := r.protectedJobs[id]
	if protected && state.state == "publishing" {
		return ErrJobPublishing
	}
	if protected {
		state.state = "revoked"
		r.protectedJobs[id] = state
		r.abandonProtectedUpload(id, state.generation)
	}
	copyJob := *job
	copyJob.Events = append(model.TaskEvents(nil), r.events[id]...)
	event := copyJob.AddEventComplete(model.JobNotification, model.CanceledNotificationStatus, "Job canceled by user")
	r.events[id] = append(r.events[id], event)
	job.Status, job.StatusPhase, job.StatusMessage, job.LastUpdate = string(event.Status), event.NotificationType, event.Message, &event.EventTime
	return nil
}

func (r *InMemoryRepository) RequeueTimedOutJob(ctx context.Context, id string, eventID int, cutoff time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.assignmentMu.Lock()
	defer r.assignmentMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return false, ErrElementNotFound
	}
	last := model.TaskEvents(r.events[id]).GetLatestPerNotificationType(model.JobNotification)
	if last == nil || last.EventID != eventID || !last.IsAssigned() || !last.EventTime.Before(cutoff) {
		return false, nil
	}
	state, protected := r.protectedJobs[id]
	if protected && (state.state == "publishing" || (state.state == "parked" && state.worker == last.WorkerName)) {
		return false, nil
	}
	if protected {
		state.state = "revoked"
		r.protectedJobs[id] = state
		r.abandonProtectedUpload(id, state.generation)
	}
	copyJob := *job
	copyJob.Events = append(model.TaskEvents(nil), r.events[id]...)
	canceled := copyJob.AddEvent(model.JobNotification, model.CanceledNotificationStatus)
	queued := copyJob.AddEvent(model.JobNotification, model.QueuedNotificationStatus)
	r.events[id] = append(r.events[id], canceled, queued)
	job.Status, job.StatusPhase, job.StatusMessage, job.LastUpdate = string(queued.Status), queued.NotificationType, queued.Message, &queued.EventTime
	return true, nil
}

// Caller holds r.mu and r.assignmentMu.
func (r *InMemoryRepository) abandonProtectedUpload(id string, generation uint64) {
	key := protectedUploadKey{id, generation}
	upload, ok := r.protectedUploads[key]
	if ok && upload.state == "staged" {
		upload.state = "abandoned"
		r.protectedUploads[key] = upload
	}
}
