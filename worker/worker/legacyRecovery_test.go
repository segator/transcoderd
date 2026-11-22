package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"transcoder/worker/job"

	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/serverclient"
)

func TestResumeRetriesPersistedNotification(t *testing.T) {
	executor, downloads, _ := newResumeTestExecutor(t)
	saved := persistResumeTestContext(t, executor.tempPath, model.JobNotification, model.StartedNotificationStatus)
	pending, err := saved.NewLegacyEvent("worker-test", model.DownloadNotification, model.StartedNotificationStatus, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := saved.StageLegacyEvent(pending); err != nil {
		t.Fatal(err)
	}
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			current := model.Job{Id: saved.JobId, Events: model.TaskEvents{
				{EventID: 0, Event: model.Event{WorkerName: "worker-test"}, NotificationType: model.JobNotification, Status: model.AssignedNotificationStatus},
				{EventID: 1, Event: model.Event{WorkerName: "worker-test"}, NotificationType: model.JobNotification, Status: model.StartedNotificationStatus},
				&pending,
			}}
			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Error(err)
			}
			return
		}
		posts++
		var envelope model.EnvelopEvent
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
			return
		}
		var got model.TaskEventType
		if err := json.Unmarshal(envelope.EventData, &got); err != nil {
			t.Error(err)
			return
		}
		if got.EventID != pending.EventID || !got.EventTime.Equal(pending.EventTime) || got.AssignmentEventID == nil || *got.AssignmentEventID != saved.EventId {
			t.Errorf("replayed event = %+v, want %+v", got, pending)
		}
	}))
	defer server.Close()
	executor.client = serverclient.NewServerClient(&web.Config{Domain: server.URL}, executor.workerConfig.Name)
	if err := executor.resumeJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || len(downloads.GetJobChan()) != 1 {
		t.Fatalf("posts=%d queued=%d", posts, len(downloads.GetJobChan()))
	}
	restored := <-downloads.GetJobChan()
	if restored.PendingLegacyEvent() != nil || restored.LastEvent.EventId != pending.EventID {
		t.Fatalf("restored state = %+v", restored.LastEvent)
	}
}

func TestRecoveryDoesNotDeleteWorkspaceOutsideWorker(t *testing.T) {
	executor, _, _ := newResumeTestExecutor(t)
	id := uuid.New()
	dir := filepath.Join(executor.tempPath, id.String())
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	victim := t.TempDir()
	j := job.NewContext(id, 0, victim)
	j.UpdateEvent(model.JobNotification, model.StartedNotificationStatus, "")
	body, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id.String()+".json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := executor.resumeJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("outside workspace affected: %v", err)
	}
}
