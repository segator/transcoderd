package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"

	"github.com/google/uuid"
)

func downloadFixture(t *testing.T, root, source string) (*Server, *scheduler.RuntimeScheduler, *repository.InMemoryRepository, *model.Job) {
	t.Helper()
	repo := repository.NewInMemoryRepository()
	job := &model.Job{Id: uuid.New(), SourcePath: source}
	if err := repo.AddJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	started := job.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	if err := repo.AddNewTaskEvent(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	s, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root, ScheduleTime: time.Hour}, repo)
	if err != nil {
		t.Fatal(err)
	}
	return NewWebServer(&Config{}, s, nil, repo), s, repo, job
}

func downloadRequest(s *Server, id uuid.UUID, writer http.ResponseWriter) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/download?uuid="+id.String(), nil)
	req.Header.Set("workerName", "worker")
	s.download(writer, req)
}

func TestSourceReadErrorBlocksOnlyCurrentAssignment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory read is used to simulate a source I/O failure")
	}
	root := t.TempDir()
	path := filepath.Join(root, "film.mkv")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	api, sched, repo, job := downloadFixture(t, root, "film.mkv")
	first := httptest.NewRecorder()
	downloadRequest(api, job.Id, first)
	if first.Code != http.StatusOK {
		t.Fatalf("first download status = %d, want 200 before read failure", first.Code)
	}
	if _, err := sched.GetChecksum(context.Background(), job.Id.String()); !errors.Is(err, scheduler.ErrorJobNotFound) {
		t.Fatalf("checksum after failed read = %v, want unavailable", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("repaired source"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := httptest.NewRecorder()
	downloadRequest(api, job.Id, second)
	if second.Code != http.StatusNotFound {
		t.Fatalf("retry status = %d, want 404", second.Code)
	}
	stored, err := repo.GetJob(context.Background(), job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := stored.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	if err := repo.AddNewTaskEvent(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	stream, err := sched.GetDownloadJobWriter(context.Background(), job.Id.String(), "worker", nil)
	if err != nil {
		t.Fatalf("new assignment could not reopen repaired source: %v", err)
	}
	_ = stream.Close(false)
}

type disconnectedWriter struct{ header http.Header }

func (w *disconnectedWriter) Header() http.Header       { return w.header }
func (w *disconnectedWriter) WriteHeader(int)           {}
func (w *disconnectedWriter) Write([]byte) (int, error) { return 0, errors.New("client disconnected") }

func TestClientDisconnectDoesNotBlockSourceOrPublishChecksum(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "film.mkv"), []byte("complete source"), 0o600); err != nil {
		t.Fatal(err)
	}
	api, sched, _, job := downloadFixture(t, root, "film.mkv")
	downloadRequest(api, job.Id, &disconnectedWriter{header: make(http.Header)})
	stream, err := sched.GetDownloadJobWriter(context.Background(), job.Id.String(), "worker", nil)
	if err != nil {
		t.Fatalf("client disconnect blocked source: %v", err)
	}
	_ = stream.Close(false)
	if _, err := sched.GetChecksum(context.Background(), job.Id.String()); !errors.Is(err, scheduler.ErrorJobNotFound) {
		t.Fatalf("checksum after disconnect = %v, want unavailable", err)
	}
}

func TestCompleteDownloadPublishesChecksum(t *testing.T) {
	tests := []struct{ name, content string }{
		{name: "multiple chunks", content: strings.Repeat("source", 30000)},
		{name: "empty source", content: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "film.mkv"), []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			api, sched, _, job := downloadFixture(t, root, "film.mkv")
			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			sched.Run(&wg, ctx)
			result := httptest.NewRecorder()
			downloadRequest(api, job.Id, result)
			cancel()
			wg.Wait()
			if result.Code != http.StatusOK || result.Body.String() != tt.content {
				t.Fatalf("download status = %d, bytes = %d", result.Code, result.Body.Len())
			}
			got, err := sched.GetChecksum(context.Background(), job.Id.String())
			want := sha256.Sum256([]byte(tt.content))
			if err != nil || got != hex.EncodeToString(want[:]) {
				t.Errorf("checksum = %q, error = %v", got, err)
			}
		})
	}
}
