package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

func TestQueuedJobsAreAuthenticatedBoundedAndOrdered(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	ctx := context.Background()
	for i := 0; i < 105; i++ {
		id := uuid.New()
		j := &model.Job{Id: id, SourcePath: fmt.Sprintf("/private/movie-%03d.mkv", i)}
		if err := repo.AddJob(ctx, j); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, Event: model.Event{EventTime: time.Unix(int64(i), 0)}, NotificationType: model.JobNotification, Status: model.QueuedNotificationStatus}); err != nil {
			t.Fatal(err)
		}
	}
	sch, err := scheduler.NewScheduler(&scheduler.Config{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWebServer(&Config{Token: "secret"}, sch, nil)
	server.ctx = ctx
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/queued", nil)
	out := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(out, req)
	if out.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", out.Code)
	}
	req.Header.Set("Authorization", "Bearer secret")
	out = httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", out.Code, out.Body.String())
	}
	var got model.QueueSnapshot
	if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 105 || len(got.Jobs) != 100 || got.Jobs[0].Name != "movie-000.mkv" || got.Jobs[99].Name != "movie-099.mkv" {
		t.Fatalf("queue = %+v", got)
	}
}
