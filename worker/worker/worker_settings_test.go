package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/serverclient"
)

type testConsoleLogger struct{}

func (testConsoleLogger) Errorf(string, ...interface{}) {}
func (testConsoleLogger) Warnf(string, ...interface{})  {}
func (testConsoleLogger) Logf(string, ...interface{})   {}
func (testConsoleLogger) Cmdf(string, ...interface{})   {}

func TestLoadPersistedWorkerSettingsBeforeAdmission(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantMax  int
	}{
		{name: "persisted zero pauses worker", response: `{"maxActiveJobs":0}`, wantMax: 0},
		{name: "persisted positive overrides startup", response: `{"maxActiveJobs":3}`, wantMax: 3},
		{name: "missing setting uses startup", response: `{}`, wantMax: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/worker/settings" {
					t.Errorf("request = %s %s, want GET worker settings", r.Method, r.URL.Path)
				}
				if r.Header.Get("workerName") != "stable-worker" {
					t.Errorf("workerName = %q, want stable-worker", r.Header.Get("workerName"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "stable-worker")
			executor := &JobExecutor{
				workerConfig:  &config.Config{MaxActiveJobs: 2},
				maxActiveJobs: 2,
				encodePool:    newEncodeConcurrency(2),
			}
			coordinator := NewServerCoordinator(client, executor, nil, testConsoleLogger{})
			executor.RequireWorkerSettings()
			if executor.AdmissionStatus().Accepting {
				t.Fatal("worker accepted jobs before persisted settings were loaded")
			}
			if !coordinator.loadPersistedWorkerSettings(context.Background()) {
				t.Fatal("loadPersistedWorkerSettings() returned false")
			}
			if got := executor.AdmissionStatus().MaxActiveJobs; got != tt.wantMax {
				t.Errorf("maximum after settings load = %d, want %d", got, tt.wantMax)
			}
		})
	}
}

func TestLoadPersistedWorkerSettingsKeepsAdmissionClosedUntilServerReturns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "stable-worker")
	executor := &JobExecutor{
		workerConfig:  &config.Config{MaxActiveJobs: 2},
		maxActiveJobs: 2,
		encodePool:    newEncodeConcurrency(2),
	}
	coordinator := NewServerCoordinator(client, executor, nil, testConsoleLogger{})
	executor.RequireWorkerSettings()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- coordinator.loadPersistedWorkerSettings(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case loaded := <-done:
		if loaded {
			t.Fatal("loadPersistedWorkerSettings() returned true while the server was unavailable")
		}
	case <-time.After(time.Second):
		t.Fatal("loadPersistedWorkerSettings() did not stop after cancellation")
	}
	if got := executor.AdmissionStatus().MaxActiveJobs; got != 2 {
		t.Errorf("unresolved settings changed configured maximum to %d, want 2", got)
	}
	if executor.AdmissionStatus().Accepting {
		t.Error("worker accepted jobs before server settings were resolved")
	}
}

func TestPersistMaxActiveJobsCommitsBeforeChangingRuntimeState(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var settings serverclient.WorkerSettings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			t.Errorf("decode settings: %v", err)
		}
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "stable-worker")
	executor := &JobExecutor{
		workerConfig:  &config.Config{MaxActiveJobs: 2},
		maxActiveJobs: 2,
		encodePool:    newEncodeConcurrency(2),
		client:        client,
	}
	if err := executor.PersistMaxActiveJobs(3); err != nil {
		t.Fatal(err)
	}
	if got := executor.AdmissionStatus().MaxActiveJobs; got != 3 {
		t.Fatalf("runtime maximum = %d, want persisted 3", got)
	}
	if _, got := executor.encodePool.Status(); got != 3 {
		t.Fatalf("encode target = %d, want 3", got)
	}

	fail.Store(true)
	if err := executor.PersistMaxActiveJobs(0); err == nil {
		t.Fatal("PersistMaxActiveJobs() succeeded while the server returned an error")
	}
	if got := executor.AdmissionStatus().MaxActiveJobs; got != 3 {
		t.Errorf("runtime maximum after persistence failure = %d, want unchanged 3", got)
	}
}

func TestLocalAdmissionDrainDoesNotPersistZero(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "stable-worker")
	executor := &JobExecutor{
		workerConfig:  &config.Config{MaxActiveJobs: 2},
		maxActiveJobs: 2,
		encodePool:    newEncodeConcurrency(2),
		client:        client,
	}
	if err := executor.DrainAcceptedJobs(); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Errorf("local drain sent %d persistence requests, want zero", requests)
	}
}

func TestManagedLimitsReplaceStaleServerSettingsBeforeAdmission(t *testing.T) {
	for _, target := range []int{0, 1, 4} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			worker := testAdmissionWorker(target)
			worker.UseManagedLimits()
			worker.RequireWorkerSettings()
			persisted := -1
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if worker.ReserveJobSlot() {
					t.Error("worker accepted a job before managed settings were synchronized")
				}
				if r.Method == http.MethodGet {
					t.Error("managed worker read stale server override")
					_, _ = w.Write([]byte(`{"maxActiveJobs":8}`))
					return
				}
				var settings serverclient.WorkerSettings
				if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
					t.Error(err)
					return
				}
				if settings.MaxActiveJobs == nil {
					t.Error("managed target missing")
					return
				}
				persisted = *settings.MaxActiveJobs
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "managed")
			coordinator := NewServerCoordinator(client, worker, nil, testConsoleLogger{})
			if !coordinator.loadPersistedWorkerSettings(context.Background()) {
				t.Fatal("managed synchronization failed")
			}
			if persisted != target || worker.AdmissionStatus().MaxActiveJobs != target {
				t.Fatalf("managed target = %d, runtime %d, want %d", persisted, worker.AdmissionStatus().MaxActiveJobs, target)
			}
			_, encodeTarget := worker.encodePool.Status()
			if encodeTarget != max(1, target) {
				t.Fatalf("encode target = %d, want %d", encodeTarget, max(1, target))
			}
		})
	}
}
