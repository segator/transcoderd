package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"transcoder/model"
	"transcoder/server/assignment"
	"transcoder/server/repository"
	"transcoder/server/scheduler"

	"github.com/google/uuid"
)

type protectedScheduler interface {
	RequestProtectedJob(context.Context, string, string, *assignment.Keyring) (*model.ProtectedAssignment, error)
	ParkProtectedJob(context.Context, string, model.AssignmentClaim) error
	ApplyProtectedEvent(context.Context, string, model.AssignmentClaim, model.TaskEventType) error
	CompleteProtectedJob(context.Context, string, model.AssignmentClaim, model.TaskEventType) error
	GetProtectedDownload(context.Context, string, model.AssignmentClaim) (*scheduler.DownloadJobStream, error)
	GetProtectedUpload(context.Context, string, model.AssignmentClaim) (*scheduler.UploadJobStream, error)
	StageProtectedUpload(context.Context, string, model.AssignmentClaim, *scheduler.UploadJobStream, string, int64) error
	GetProtectedChecksum(context.Context, string, model.AssignmentClaim) (string, error)
}

func (p *protectedAccess) notify(w http.ResponseWriter, r *http.Request) {
	worker, ok := p.authenticate(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	var input struct {
		Claim model.AssignmentClaim `json:"claim"`
		Event model.TaskEventType   `json:"event"`
	}
	if err := decodeDashboardJSON(w, r, &input); err != nil || (input.Event.WorkerName != "" && input.Event.WorkerName != worker) {
		http.Error(w, "invalid protected event", http.StatusBadRequest)
		return
	}
	input.Event.WorkerName = worker
	var err error
	if input.Event.NotificationType == model.JobNotification && input.Event.Status == model.CompletedNotificationStatus {
		err = p.scheduler.CompleteProtectedJob(r.Context(), worker, input.Claim, input.Event)
	} else {
		err = p.scheduler.ApplyProtectedEvent(r.Context(), worker, input.Claim, input.Event)
	}
	if err != nil {
		if errors.Is(err, repository.ErrStaleAssignment) || errors.Is(err, repository.ErrEventConflict) {
			http.Error(w, "protected event conflicts with current assignment", http.StatusConflict)
			return
		}
		http.Error(w, "could not record protected event", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type protectedAccess struct {
	credentials *assignment.Credentials
	keys        *assignment.Keyring
	scheduler   protectedScheduler
}

func newProtectedAccess(config *Config, scheduler protectedScheduler) (*protectedAccess, error) {
	if config == nil || scheduler == nil || len(config.ProtectedWorkerHashes) == 0 {
		return nil, errors.New("protected assignment access is not configured")
	}
	credentials, err := assignment.NewCredentials(config.ProtectedWorkerHashes)
	if err != nil {
		return nil, err
	}
	keys := make(map[string][]byte, len(config.AssignmentSigningKeys))
	for id, encoded := range config.AssignmentSigningKeys {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid assignment signing key %q", id)
		}
		keys[id] = key
	}
	keyring, err := assignment.NewKeyring(config.ActiveAssignmentKey, keys)
	if err != nil {
		return nil, err
	}
	return &protectedAccess{credentials: credentials, keys: keyring, scheduler: scheduler}, nil
}

// These handlers are deliberately not registered until protected events and
// transfers are implemented. A partially protected protocol must not issue a
// job that can only be completed through an unsafe legacy route.
func (p *protectedAccess) authenticate(w http.ResponseWriter, r *http.Request) (string, bool) {
	worker := workerNameFromRequest(r)
	if err := p.credentials.Authenticate(worker, r.Header.Get("X-Worker-Credential")); err != nil {
		writeUnauthorized(w)
		return "", false
	}
	w.Header().Set("Cache-Control", "no-store")
	return worker, true
}

func (p *protectedAccess) requestJob(w http.ResponseWriter, r *http.Request) {
	worker, ok := p.authenticate(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	var input struct {
		RequestID string `json:"requestId"`
	}
	if err := decodeDashboardJSON(w, r, &input); err != nil || input.RequestID == "" {
		http.Error(w, "invalid request identity", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(input.RequestID); err != nil {
		http.Error(w, "invalid request identity", http.StatusBadRequest)
		return
	}
	result, err := p.scheduler.RequestProtectedJob(r.Context(), worker, input.RequestID, p.keys)
	if err != nil {
		http.Error(w, "could not request protected job", http.StatusInternalServerError)
		return
	}
	if result == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		return
	}
}

func (p *protectedAccess) parkJob(w http.ResponseWriter, r *http.Request) {
	worker, ok := p.authenticate(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	var claim model.AssignmentClaim
	if err := decodeDashboardJSON(w, r, &claim); err != nil {
		http.Error(w, "invalid assignment claim", http.StatusBadRequest)
		return
	}
	if err := p.scheduler.ParkProtectedJob(r.Context(), worker, claim); err != nil {
		if errors.Is(err, repository.ErrStaleAssignment) {
			http.Error(w, "assignment is no longer current", http.StatusConflict)
			return
		}
		http.Error(w, "could not park assignment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func protectedTransferClaim(r *http.Request) (model.AssignmentClaim, error) {
	id, err := uuid.Parse(r.URL.Query().Get("uuid"))
	if err != nil || id == uuid.Nil {
		return model.AssignmentClaim{}, errors.New("invalid job identity")
	}
	generation, err := strconv.ParseUint(r.Header.Get("X-Assignment-Generation"), 10, 64)
	capability := r.Header.Get("X-Assignment-Capability")
	if err != nil || generation == 0 || capability == "" || len(capability) > 256 {
		return model.AssignmentClaim{}, errors.New("invalid assignment identity")
	}
	return model.AssignmentClaim{JobID: id, Generation: generation, Capability: capability}, nil
}

func protectedTransferError(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrStaleAssignment) {
		http.Error(w, "assignment is no longer current", http.StatusConflict)
		return
	}
	if errors.Is(err, scheduler.ErrorJobNotFound) {
		http.Error(w, "source is unavailable", http.StatusNotFound)
		return
	}
	http.Error(w, "protected transfer failed", http.StatusInternalServerError)
}

func (p *protectedAccess) download(w http.ResponseWriter, r *http.Request) {
	worker, ok := p.authenticate(w, r)
	if !ok {
		return
	}
	claim, err := protectedTransferClaim(r)
	if err != nil {
		http.Error(w, "invalid assignment", http.StatusBadRequest)
		return
	}
	stream, err := p.scheduler.GetProtectedDownload(r.Context(), worker, claim)
	if err != nil {
		protectedTransferError(w, err)
		return
	}
	defer stream.Close(true)
	w.Header().Set("Content-Length", strconv.FormatInt(stream.Size(), 10))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(stream.Name())))
	_, _ = io.Copy(w, stream)
}

func (p *protectedAccess) checksum(w http.ResponseWriter, r *http.Request) {
	worker, ok := p.authenticate(w, r)
	if !ok {
		return
	}
	claim, err := protectedTransferClaim(r)
	if err != nil {
		http.Error(w, "invalid assignment", http.StatusBadRequest)
		return
	}
	value, err := p.scheduler.GetProtectedChecksum(r.Context(), worker, claim)
	if err != nil {
		protectedTransferError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, value)
}

func (p *protectedAccess) upload(w http.ResponseWriter, r *http.Request) {
	worker, ok := p.authenticate(w, r)
	if !ok {
		return
	}
	claim, err := protectedTransferClaim(r)
	if err != nil {
		http.Error(w, "invalid assignment", http.StatusBadRequest)
		return
	}
	stream, err := p.scheduler.GetProtectedUpload(r.Context(), worker, claim)
	if err != nil {
		protectedTransferError(w, err)
		return
	}
	receiveUpload(w, r, stream, func(checksum string, size int64) error {
		return p.scheduler.StageProtectedUpload(r.Context(), worker, claim, stream, checksum, size)
	})
}
