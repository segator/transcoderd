package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"

	"github.com/google/uuid"
)

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
	complete := current.AddEvent(model.JobNotification, model.CompletedNotificationStatus)
	complete.WorkerName = "node"
	completeBody, _ := json.Marshal(struct {
		Claim model.AssignmentClaim `json:"claim"`
		Event model.TaskEventType   `json:"event"`
	}{Claim: claimValue, Event: *complete})
	if got := request(access.notify, string(completeBody), "node", token).Code; got != http.StatusConflict {
		t.Fatalf("completion without staged output = %d, want 409", got)
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
