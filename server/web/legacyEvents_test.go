package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

func TestWorkerNotificationRejectionHeaders(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	id := uuid.New()
	if err := repo.AddJob(context.Background(), &model.Job{Id: id, SourcePath: "movie.mkv", TargetPath: "out.mkv"}); err != nil {
		t.Fatal(err)
	}
	for seq, status := range []model.NotificationStatus{model.AssignedNotificationStatus, model.StartedNotificationStatus} {
		if err := repo.AddNewTaskEvent(context.Background(), &model.TaskEventType{JobId: id, EventID: seq, Event: model.Event{WorkerName: "worker"}, NotificationType: model.JobNotification, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	sched, err := scheduler.NewScheduler(&scheduler.Config{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	api := NewWebServer(&Config{}, sched, nil, repo)
	api.ctx = context.Background()
	assignment := 0
	cases := []struct {
		name  string
		event model.TaskEventType
		code  string
	}{
		{"expired", model.TaskEventType{JobId: id, EventID: 2, Event: model.Event{WorkerName: "other"}, AssignmentEventID: &assignment, NotificationType: model.DownloadNotification, Status: model.StartedNotificationStatus}, "assignment-expired"},
		{"conflict", model.TaskEventType{JobId: id, EventID: 4, Event: model.Event{WorkerName: "worker"}, AssignmentEventID: &assignment, NotificationType: model.DownloadNotification, Status: model.StartedNotificationStatus}, "event-conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.event)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := json.Marshal(model.EnvelopEvent{EventType: model.NotificationEvent, EventData: raw})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/event", bytes.NewReader(envelope))
			response := httptest.NewRecorder()
			api.handleWorkerEvent(response, request)
			if response.Code != http.StatusConflict || response.Header().Get("X-TranscoderD-Error") != tc.code {
				t.Fatalf("response = %d %q", response.Code, response.Header().Get("X-TranscoderD-Error"))
			}
		})
	}
}
