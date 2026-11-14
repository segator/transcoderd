package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"transcoder/model"
	"transcoder/server/library"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/update"
)

func mediaTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "clip.mkv"), []byte("0123456789abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root}, repo)
	if err != nil {
		t.Fatal(err)
	}
	updater, err := update.NewUpdater("0.0.0", "test", true, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewWebServer(&Config{Token: "worker-secret", DashboardToken: "encoding123", MediaRoots: map[string]string{"encoded": t.TempDir()}}, sch, updater, repo)
	t.Cleanup(s.library.Close)
	return s, root
}
func TestMediaAuthRangesAndContainment(t *testing.T) {
	s, root := mediaTestServer(t)
	id := library.FileID("source", "clip.mkv")
	cookie := loginDashboard(t, s)
	for _, path := range []string{"roots", "browse", "info/" + id, "file/" + id, "preview/" + id, "jobs/00000000-0000-0000-0000-000000000000", "operations/missing"} {
		if got := dashboardRequest(s, "GET", "/dashboard/api/media/"+path, "", "", nil); got.Code != 401 {
			t.Errorf("anonymous %s = %d", path, got.Code)
		}
	}
	req := httptest.NewRequest("GET", "/dashboard/api/media/file/"+id, nil)
	req.AddCookie(cookie)
	req.Header.Set("Range", "bytes=2-6")
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "23456" || w.Header().Get("Content-Range") != "bytes 2-6/16" {
		t.Fatalf("range = %d %q %+v", w.Code, w.Body.String(), w.Header())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("media response is cacheable")
	}
	if err := os.Symlink("/etc", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../secret.mkv", ".transcoderd/staged.mkv", "escape/passwd.mkv"} {
		got := dashboardRequest(s, "GET", "/dashboard/api/media/file/"+library.FileID("source", p), "", "", cookie)
		if got.Code != 400 {
			t.Errorf("unsafe %s = %d", p, got.Code)
		}
	}
	if got := dashboardRequest(s, "GET", "/dashboard/api/media/jobs/00000000-0000-0000-0000-000000000000", "", "", cookie); got.Code != 404 {
		t.Errorf("unknown job = %d", got.Code)
	}
	if got := dashboardRequest(s, "GET", "/dashboard/api/media/preview/"+id+"?start=NaN", "", "", cookie); got.Code != 400 {
		t.Errorf("nonfinite seek = %d", got.Code)
	}
	if got := dashboardRequest(s, "GET", "/dashboard/api/media/browse?path=..", "", "", cookie); got.Code != 400 {
		t.Errorf("unsafe folder = %d", got.Code)
	}
}
func TestMediaMutationValidationAndReadOnlyRoots(t *testing.T) {
	s, _ := mediaTestServer(t)
	cookie := loginDashboard(t, s)
	req := httptest.NewRequest("POST", "/dashboard/api/media/scans", strings.NewReader(`{}`))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("missing CSRF header = %d", w.Code)
	}
	tests := []struct {
		path, body string
		want       int
	}{
		{"scans", `{"candidateLimit":5001}`, 400},
		{"scans", `{"path":"../"}`, 400},
		{"scans", `{"unknown":true}`, 400},
		{"batches", `{"scanId":"missing","ids":["anything"],"target":1}`, 404},
		{"batches", `{"target":0}`, 400},
	}
	for _, tt := range tests {
		got := dashboardRequest(s, "POST", "/dashboard/api/media/"+tt.path, tt.body, "", cookie)
		if got.Code != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.path, tt.body, got.Code, tt.want)
		}
	}
	got := dashboardRequest(s, "GET", "/dashboard/api/media/roots", "", "", cookie)
	var roots struct{ Roots []model.MediaRoot }
	if err := json.Unmarshal(got.Body.Bytes(), &roots); err != nil {
		t.Fatal(err)
	}
	if len(roots.Roots) != 2 || !roots.Roots[0].Queueable || roots.Roots[1].Queueable {
		t.Fatalf("roots = %+v", roots)
	}
	// Closing the library must cancel asynchronous work without needing a request.
	s.library.Close()
	_, err := s.library.Info(context.Background(), library.FileID("encoded", "missing.mkv"))
	if err == nil {
		t.Error("missing file available")
	}
}

func TestMediaJobPlaybackPolicy(t *testing.T) {
	s, root := mediaTestServer(t)
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root}, repo)
	if err != nil {
		t.Fatal(err)
	}
	s.library.Close()
	s.library = library.New(sch, s.config.MediaRoots, 2)
	cookie := loginDashboard(t, s)
	tests := []struct {
		state model.NotificationStatus
		want  int
	}{
		{model.QueuedNotificationStatus, 200}, {model.StartedNotificationStatus, 409},
		{model.FailedNotificationStatus, 409}, {model.CanceledNotificationStatus, 409}, {model.CompletedNotificationStatus, 200},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			j := &model.Job{Id: uuid.New(), SourcePath: "clip.mkv", TargetPath: "missing-output.mkv"}
			if err := repo.AddJob(t.Context(), j); err != nil {
				t.Fatal(err)
			}
			if err := repo.AddNewTaskEvent(t.Context(), j.AddEvent(model.JobNotification, tt.state)); err != nil {
				t.Fatal(err)
			}
			w := dashboardRequest(s, "GET", "/dashboard/api/media/jobs/"+j.Id.String(), "", "", cookie)
			if w.Code != tt.want {
				t.Fatalf("mediaJobAssets() = %d %s, want %d", w.Code, w.Body.String(), tt.want)
			}
			if tt.state == model.CompletedNotificationStatus && strings.TrimSpace(w.Body.String()) != `{"files":[]}` {
				t.Fatalf("completed missing output substituted source: %s", w.Body.String())
			}
		})
	}
}
