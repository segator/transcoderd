package serverclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"transcoder/server/web"
)

func TestWorkerSettingsClientReadsAndPersistsLimit(t *testing.T) {
	var saved WorkerSettings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-test-token" {
			t.Errorf("Authorization = %q, want bearer token", r.Header.Get("Authorization"))
		}
		if r.Header.Get("workerName") != "gpu-worker" {
			t.Errorf("workerName = %q, want gpu-worker", r.Header.Get("workerName"))
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(WorkerSettings{MaxActiveJobs: intPointer(0)})
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Errorf("decode PUT body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("method = %s, want GET or PUT", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := NewServerClient(&web.Config{Domain: server.URL, Token: "secret-test-token"}, "gpu-worker")
	settings, err := client.GetWorkerSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxActiveJobs == nil || *settings.MaxActiveJobs != 0 {
		t.Fatalf("GetWorkerSettings() = %#v, want persisted zero", settings)
	}
	if err := client.SetWorkerMaxActiveJobs(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if saved.MaxActiveJobs == nil || *saved.MaxActiveJobs != 3 {
		t.Fatalf("saved maxActiveJobs = %v, want 3", saved.MaxActiveJobs)
	}
}

func TestWorkerSettingsClientRejectsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("database unavailable"))
	}))
	defer server.Close()

	client := NewServerClient(&web.Config{Domain: server.URL, Token: "test"}, "worker")
	if _, err := client.GetWorkerSettings(context.Background()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("GetWorkerSettings() error = %v, want server status error", err)
	}
	if err := client.SetWorkerMaxActiveJobs(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("SetWorkerMaxActiveJobs() error = %v, want server status error", err)
	}
}

func TestGetWorkerSettingsHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewServerClient(&web.Config{Domain: server.URL}, "worker")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.GetWorkerSettings(ctx)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("settings request did not reach the server")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("GetWorkerSettings() returned no error after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("GetWorkerSettings() did not stop after cancellation")
	}
}

func intPointer(value int) *int { return &value }
