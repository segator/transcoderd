package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"

	"github.com/google/uuid"
)

func TestProtectedTransferHandlersFenceCredentialsAndCancellation(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	source := []byte("source movie")
	if err := os.WriteFile(filepath.Join(root, "film.mkv"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root}, repo)
	if err != nil {
		t.Fatal(err)
	}
	token := "node-credential"
	digest := sha256.Sum256([]byte(token))
	access, err := newProtectedAccess(&Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))}, ActiveAssignmentKey: "v1"}, sch)
	if err != nil {
		t.Fatal(err)
	}
	video := &model.Job{Id: uuid.New(), SourcePath: "film.mkv", TargetPath: "target.mkv"}
	if err := repo.AddJob(ctx, video); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, video.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "node", uuid.NewString(), access.keys)
	if err != nil || assigned == nil {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	claim := model.AssignmentClaim{JobID: video.Id, Generation: assigned.Generation, Capability: assigned.Capability, InstanceID: uuid.New()}
	if _, err := repo.AcquireProtectedLease(ctx, "node", claim, claim.InstanceID, protectedLeaseDuration); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetJob(ctx, video.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := current.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "node"
	if err := repo.ApplyProtectedEvent(ctx, "node", claim, *started); err != nil {
		t.Fatal(err)
	}
	request := func(handler http.HandlerFunc, method string, body []byte, credential string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/protected?uuid="+video.Id.String(), bytes.NewReader(body))
		r.Header.Set("workerName", "node")
		r.Header.Set("X-Worker-Credential", credential)
		r.Header.Set("X-Assignment-Generation", strconv.FormatUint(claim.Generation, 10))
		r.Header.Set("X-Assignment-Capability", claim.Capability)
		r.Header.Set("X-Worker-Instance", claim.InstanceID.String())
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
		sum := sha256.Sum256(body)
		r.Header.Set("checksum", hex.EncodeToString(sum[:]))
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	if response := request(access.download, http.MethodGet, nil, token); response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), source) {
		t.Fatalf("download = %d %q", response.Code, response.Body.Bytes())
	}
	checksum := sha256.Sum256(source)
	if response := request(access.checksum, http.MethodGet, nil, token); response.Code != http.StatusOK || response.Body.String() != hex.EncodeToString(checksum[:]) {
		t.Fatalf("checksum = %d %q", response.Code, response.Body.String())
	}
	if response := request(access.upload, http.MethodPost, []byte("encoded"), "wrong"); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated upload = %d", response.Code)
	}
	if response := request(access.upload, http.MethodPost, []byte("encoded"), token); response.Code != http.StatusCreated {
		t.Fatalf("protected upload = %d %q", response.Code, response.Body.String())
	}
	if err := repo.CancelJobAssignment(ctx, video.Id.String()); err != nil {
		t.Fatal(err)
	}
	if response := request(access.checksum, http.MethodGet, nil, token); response.Code != http.StatusConflict {
		t.Fatalf("checksum after cancellation = %d", response.Code)
	}
}

func TestProtectedAssignmentHandlers(t *testing.T) {
	ctx := t.Context()
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	token := "a-specific-worker-credential"
	digest := sha256.Sum256([]byte(token))
	config := &Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))}, ActiveAssignmentKey: "v1"}
	access, err := newProtectedAccess(config, sch)
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: "film.mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	request := func(handler http.HandlerFunc, body, worker, credential string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/protected", strings.NewReader(body))
		r.Header.Set("workerName", worker)
		r.Header.Set("X-Worker-Credential", credential)
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	body := `{"requestId":"` + requestID + `"}`
	if got := request(access.requestJob, body, "other", token).Code; got != http.StatusUnauthorized {
		t.Fatalf("request with another identity = %d, want 401", got)
	}
	response := request(access.requestJob, body, "node", token)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("request = %d %s", response.Code, response.Body.String())
	}
	var assigned model.ProtectedAssignment
	if err := json.Unmarshal(response.Body.Bytes(), &assigned); err != nil {
		t.Fatal(err)
	}
	retried := request(access.requestJob, body, "node", token)
	if retried.Code != http.StatusOK || retried.Body.String() != response.Body.String() {
		t.Fatalf("retried request = %d %s, want same result", retried.Code, retried.Body.String())
	}
	current, err := repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := current.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "node"
	claimValue := model.AssignmentClaim{JobID: assigned.Id, Generation: assigned.Generation, Capability: assigned.Capability, InstanceID: uuid.New()}
	claim, _ := json.Marshal(claimValue)
	if got := request(access.lease, string(claim), "node", token).Code; got != http.StatusOK {
		t.Fatalf("acquire lease = %d, want 200", got)
	}
	otherInstance := claimValue
	otherInstance.InstanceID = uuid.New()
	otherClaim, _ := json.Marshal(otherInstance)
	if got := request(access.lease, string(otherClaim), "node", token).Code; got != http.StatusConflict {
		t.Fatalf("acquire conflicting lease = %d, want 409", got)
	}
	eventBody, _ := json.Marshal(struct {
		Claim model.AssignmentClaim `json:"claim"`
		Event model.TaskEventType   `json:"event"`
	}{Claim: claimValue, Event: *started})
	for i := 0; i < 2; i++ {
		if got := request(access.notify, string(eventBody), "node", token).Code; got != http.StatusNoContent {
			t.Fatalf("event attempt %d = %d, want 204", i, got)
		}
	}
	progressBody, _ := json.Marshal(struct {
		Claim    model.AssignmentClaim  `json:"claim"`
		Progress model.TaskProgressType `json:"progress"`
		Sequence uint64                 `json:"sequence"`
	}{Claim: claimValue, Progress: model.TaskProgressType{JobId: job.Id, ProgressID: "download", NotificationType: model.DownloadNotification, Status: model.ProgressingTaskProgressTypeStatus, Percent: 20}, Sequence: 1})
	if got := request(access.progress, string(progressBody), "node", token).Code; got != http.StatusNoContent {
		t.Fatalf("progress = %d, want 204", got)
	}
	complete := current.AddEvent(model.JobNotification, model.CompletedNotificationStatus)
	complete.WorkerName = "node"
	completeBody, _ := json.Marshal(struct {
		Claim model.AssignmentClaim `json:"claim"`
		Event model.TaskEventType   `json:"event"`
	}{Claim: claimValue, Event: *complete})
	if got := request(access.notify, string(completeBody), "node", token).Code; got != http.StatusConflict {
		t.Fatalf("completion without staged output = %d, want 409", got)
	}
	if got := request(access.parkJob, string(claim), "node", token).Code; got != http.StatusConflict {
		t.Fatalf("park before prefetch = %d, want 409", got)
	}
	current, err = repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []model.NotificationStatus{model.StartedNotificationStatus, model.CompletedNotificationStatus} {
		event := current.AddEvent(model.DownloadNotification, status)
		event.WorkerName = "node"
		if err := repo.ApplyProtectedEvent(ctx, "node", claimValue, *event); err != nil {
			t.Fatal(err)
		}
	}
	if got := request(access.parkJob, string(claim), "node", token).Code; got != http.StatusNoContent {
		t.Fatalf("park = %d, want 204", got)
	}
	if got := request(access.parkJob, string(claim), "node", token).Code; got != http.StatusNoContent {
		t.Fatalf("repeated park = %d, want 204", got)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); err != nil {
		t.Fatal(err)
	}
	if got := request(access.parkJob, string(claim), "node", token).Code; got != http.StatusConflict {
		t.Fatalf("park after cancel = %d, want 409", got)
	}
	if got := request(access.notify, string(eventBody), "node", token).Code; got != http.StatusConflict {
		t.Fatalf("event after cancel = %d, want 409", got)
	}
}
