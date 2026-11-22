package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"
)

func TestStalledNotificationDoesNotBlockAnotherJob(t *testing.T) {
	root := t.TempDir()
	first := job.NewContext(uuid.New(), 0, filepath.Join(root, "first"))
	second := job.NewContext(uuid.New(), 0, filepath.Join(root, "second"))
	if err := first.Init(); err != nil {
		t.Fatal(err)
	}
	if err := second.Init(); err != nil {
		t.Fatal(err)
	}
	firstReceived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope model.EnvelopEvent
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
			return
		}
		var event model.TaskEventType
		if err := json.Unmarshal(envelope.EventData, &event); err != nil {
			t.Error(err)
			return
		}
		if event.JobId == first.JobId {
			close(firstReceived)
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	e := &JobExecutor{serviceCtx: ctx, workerConfig: &config.Config{Name: "worker"}, console: console.NewRenderService(), client: serverclient.NewServerClient(&web.Config{Domain: server.URL}, "worker")}
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- e.publishTaskEvent(first, model.DownloadNotification, model.StartedNotificationStatus, "")
	}()
	select {
	case <-firstReceived:
	case <-time.After(time.Second):
		t.Fatal("first notification not sent")
	}
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- e.publishTaskEvent(second, model.DownloadNotification, model.StartedNotificationStatus, "")
	}()
	select {
	case err := <-secondResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("second job blocked behind first")
	}
	cancel()
	select {
	case err := <-firstResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first did not stop")
	}
	if first.PendingLegacyEvent() == nil {
		t.Fatal("interrupted event was not retained")
	}
}

func TestBlockedNotificationDoesNotBlockSamePhase(t *testing.T) {
	root := t.TempDir()
	first := job.NewContext(uuid.New(), 0, filepath.Join(root, "first"))
	second := job.NewContext(uuid.New(), 0, filepath.Join(root, "second"))
	if err := first.Init(); err != nil {
		t.Fatal(err)
	}
	if err := second.Init(); err != nil {
		t.Fatal(err)
	}
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope model.EnvelopEvent
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
			return
		}
		if envelope.EventType != model.NotificationEvent {
			return
		}
		var event model.TaskEventType
		if err := json.Unmarshal(envelope.EventData, &event); err != nil {
			t.Error(err)
			return
		}
		if event.JobId == first.JobId {
			close(received)
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executed := make(chan struct{}, 1)
	e := &JobExecutor{serviceCtx: ctx, workerConfig: &config.Config{Name: "worker"}, console: console.NewRenderService(), client: serverclient.NewServerClient(&web.Config{Domain: server.URL}, "worker")}
	executor := step.NewStepExecutor(model.DownloadNotification, func(j *job.Context) []step.Action {
		return []step.Action{{Id: j.JobId.String(), Execute: func(context.Context, step.Tracker) error {
			if j.JobId == second.JobId {
				executed <- struct{}{}
			}
			return nil
		}}}
	})
	executor.AddJob(first)
	go e.stepQueueRoutine(ctx, executor)
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("first event not sent")
	}
	executor.AddJob(second)
	select {
	case <-executed:
	case <-time.After(2 * time.Second):
		t.Fatal("second job blocked by first event")
	}
	cancel()
}
