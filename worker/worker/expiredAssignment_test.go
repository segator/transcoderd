package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"
)

func TestExpiredAssignmentDiscardsOnlyAffectedJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TranscoderD-Error", "assignment-expired")
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	root := t.TempDir()
	j := job.NewContext(uuid.New(), 2, filepath.Join(root, "expired"))
	other := filepath.Join(root, "other")
	if err := j.Init(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	e := &JobExecutor{workerConfig: &config.Config{Name: "worker"}, console: console.NewRenderService(), client: serverclient.NewServerClient(&web.Config{Domain: server.URL}, "worker"), activeJobs: 2}
	var ran atomic.Bool
	executor := step.NewStepExecutor(model.DownloadNotification, func(*job.Context) []step.Action {
		return []step.Action{{Id: "work", Execute: func(context.Context, step.Tracker) error { ran.Store(true); return nil }}}
	})
	e.processStep(context.Background(), executor, j)
	if ran.Load() {
		t.Fatal("action ran after rejection")
	}
	if _, err := os.Stat(j.WorkingDir); !os.IsNotExist(err) {
		t.Fatalf("expired workspace still exists: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated workspace changed: %v", err)
	}
	if got := e.ActiveJobs(); got != 1 {
		t.Fatalf("active jobs = %d, want 1", got)
	}
}

func TestConflictingEventStopsJobAndPreservesWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TranscoderD-Error", "event-conflict")
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	j := job.NewContext(uuid.New(), 2, filepath.Join(t.TempDir(), "rejected"))
	if err := j.Init(); err != nil {
		t.Fatal(err)
	}
	e := &JobExecutor{workerConfig: &config.Config{Name: "worker"}, console: console.NewRenderService(), client: serverclient.NewServerClient(&web.Config{Domain: server.URL}, "worker"), activeJobs: 1}
	var ran atomic.Bool
	executor := step.NewStepExecutor(model.DownloadNotification, func(*job.Context) []step.Action {
		return []step.Action{{Id: "work", Execute: func(context.Context, step.Tracker) error { ran.Store(true); return nil }}}
	})
	e.processStep(context.Background(), executor, j)
	if ran.Load() {
		t.Fatal("action ran after rejection")
	}
	if got := e.ActiveJobs(); got != 0 {
		t.Fatalf("active jobs = %d, want 0", got)
	}
	saved, err := job.ReadContextFromDiskByPath(filepath.Join(j.WorkingDir, j.JobId.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.RejectedReason == "" || saved.PendingLegacyEvent() == nil {
		t.Fatalf("missing rejection diagnostic: %+v", saved)
	}
}
