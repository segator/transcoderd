package web

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

func TestRemoteControlAuthenticationAndCapabilities(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	s := NewWebServer(&Config{Token: "worker-secret", DashboardToken: "encoding123"}, sch, nil)
	publish := func(runtime *model.WorkerRuntime) {
		t.Helper()
		if err := repo.PingServerUpdate(context.Background(), "127.0.0.1", model.PingEventType{Event: model.Event{WorkerName: "remote"}, Runtime: runtime}); err != nil {
			t.Fatal(err)
		}
	}
	runtime := &model.WorkerRuntime{ControlVersion: 1, CPUJobs: 2, CapacityVersion: 2}
	publish(runtime)
	path := "/dashboard/api/workers/remote/control"
	tests := []struct {
		name, token, body string
		want              int
	}{
		{"anonymous", "", `{"action":"pause-encodes"}`, 401},
		{"worker credentials", "worker-secret", `{"action":"pause-encodes"}`, 401},
		{"invalid action", "encoding123", `{"action":"shutdown"}`, 400},
		{"unknown fields", "encoding123", `{"action":"pause-encodes","unexpected":true}`, 400},
		{"valid", "encoding123", `{"action":"pause-encodes"}`, 202},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := dashboardRequest(s, "POST", path, tt.body, tt.token, nil)
			if w.Code != tt.want {
				t.Fatalf("control = %d %s, want %d", w.Code, w.Body.String(), tt.want)
			}
		})
	}
	req := httptest.NewRequest("POST", path, strings.NewReader(`{"action":"pause-encodes"}`))
	req.Header.Set("Authorization", "Bearer encoding123")
	req.Header.Set("X-Dashboard-Request", "1")
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("cross-origin control = %d", w.Code)
	}
	for _, rt := range []*model.WorkerRuntime{nil, {ControlVersion: 0}, {ControlVersion: 1, Draining: true}} {
		publish(rt)
		w := dashboardRequest(s, "POST", path, `{"action":"pause-encodes"}`, "encoding123", nil)
		if w.Code != 409 {
			t.Fatalf("unsupported control = %d %s", w.Code, w.Body.String())
		}
	}
	publish(runtime)
	old := dashboardRequest(s, "GET", "/api/v1/worker/settings", "", "worker-secret", nil)
	if old.Code != 400 {
		t.Fatalf("worker without name = %d", old.Code)
	}
	get := func(capable bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/worker/settings", nil)
		r.Header.Set("Authorization", "Bearer worker-secret")
		r.Header.Set("workerName", "remote")
		r.Header.Set("workerCapabilities", model.IndependentEncoderCapability)
		if capable {
			r.Header.Set("workerControlVersion", model.RemoteControlCapability)
		}
		response := httptest.NewRecorder()
		s.srv.Handler.ServeHTTP(response, r)
		return response
	}
	if response := get(false); strings.Contains(response.Body.String(), `"control"`) {
		t.Fatal("legacy settings exposed remote command")
	}
	response := get(true)
	var settings model.WorkerSettings
	if err := json.Unmarshal(response.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Control == nil || settings.Control.State != "pending" {
		t.Fatalf("capable settings = %s", response.Body.String())
	}
	if response := dashboardRequest(s, "POST", "/api/v1/worker/control/ack", `{"id":"`+settings.Control.ID+`"}`, "encoding123", nil); response.Code != 401 {
		t.Fatalf("dashboard credentials acknowledged worker command: %d", response.Code)
	}
}

func TestIndividualRemoteControlValidationAndDelivery(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWebServer(&Config{Token: "worker-secret", DashboardToken: "dashboard"}, sch, nil)
	id := uuid.NewString()
	runtime := &model.WorkerRuntime{ControlVersion: 2, CPUJobs: 2, Jobs: []model.WorkerJobRuntime{{ID: id, Phase: "FFMPEG", State: "running", CanPause: true}}}
	publish := func() {
		t.Helper()
		if err := repo.PingServerUpdate(t.Context(), "127.0.0.1", model.PingEventType{Event: model.Event{WorkerName: "remote"}, Runtime: runtime}); err != nil {
			t.Fatal(err)
		}
	}
	publish()
	tests := []struct {
		action, id, token string
		want              int
	}{
		{"pause-job", id, "", 401}, {"pause-job", id, "worker-secret", 401},
		{"pause-job", "bad", "dashboard", 400}, {"pause-job", "", "dashboard", 400},
		{"pause-job", uuid.NewString(), "dashboard", 409}, {"pause-encodes", id, "dashboard", 400},
		{"pause-job", id, "dashboard", 202}, {"resume-job", id, "dashboard", 202},
	}
	for _, tt := range tests {
		body, _ := json.Marshal(map[string]string{"action": tt.action, "jobId": tt.id})
		w := dashboardRequest(server, "POST", "/dashboard/api/workers/remote/control", string(body), tt.token, nil)
		if w.Code != tt.want {
			t.Errorf("%s %s=%d %s,want %d", tt.action, tt.id, w.Code, w.Body.String(), tt.want)
		}
	}
	for _, version := range []string{"", "1", "2"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/worker/settings", nil)
		r.Header.Set("Authorization", "Bearer worker-secret")
		r.Header.Set("workerName", "remote")
		r.Header.Set("workerControlVersion", version)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), `"jobId"`) != (version == "2") {
			t.Fatalf("version %q delivery: %s", version, w.Body.String())
		}
	}
	runtime.ControlVersion = 1
	publish()
	if _, err := sch.ControlWorker(t.Context(), "remote", "pause-job", id); err == nil {
		t.Fatal("v1 accepted job control")
	}
	if _, err := sch.ControlWorker(t.Context(), "remote", "pause-encodes"); err != nil {
		t.Fatal("v1 lost worker control", err)
	}
	runtime.ControlVersion = 2
	runtime.Jobs[0].CanPause = false
	publish()
	if _, err := sch.ControlWorker(t.Context(), "remote", "pause-job", id); err == nil {
		t.Fatal("noncontrollable task accepted")
	}
	runtime.Jobs[0].CanPause = true
	runtime.Jobs[0].Phase = "Upload"
	publish()
	if _, err := sch.ControlWorker(t.Context(), "remote", "pause-job", id); err == nil {
		t.Fatal("nonencoding task accepted")
	}
}
