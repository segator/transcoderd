package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

func TestJobIntakeAcceptsTransportStreams(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	for _, name := range []string{"concert.ts", "episode.m2ts", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root, MinFileSize: 1}, repo)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWebServer(&Config{Token: "test-token"}, sch, nil, repo)
	server.ctx = ctx

	req := httptest.NewRequest(http.MethodPost, "/api/v1/job/", strings.NewReader("{\"sourcePath\":\".\"}"))
	req.Header.Set("Authorization", "Bearer test-token")
	out := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("job submission status = %d, want 200: %s", out.Code, out.Body.String())
	}

	var got scheduler.ScheduleJobRequestResult
	if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ScheduledJobs) != 2 || len(got.FailedJobRequest) != 1 || len(got.SkippedFiles) != 0 {
		t.Fatalf("job submission = %s, want two scheduled jobs and one rejection", out.Body.String())
	}
	if failed := got.FailedJobRequest[0]; failed.SourcePath != "notes.txt" || !strings.Contains(failed.Error, "Invalid Extension txt") {
		t.Errorf("rejected path and reason = %q, %q, want notes.txt with invalid extension", failed.SourcePath, failed.Error)
	}

	for source, target := range map[string]string{"concert.ts": "concert.mkv", "episode.m2ts": "episode.mkv"} {
		job, err := repo.GetJobByPath(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		if job == nil {
			t.Fatalf("missing queued job for %q", source)
		}
		if status := job.Events.GetStatus(); status != model.QueuedNotificationStatus || job.TargetPath != target {
			t.Errorf("%q status and destination = %q, %q, want queued and %q", source, status, job.TargetPath, target)
		}
	}
}
