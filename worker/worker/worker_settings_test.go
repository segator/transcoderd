package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/model"
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

func TestSynchronizeRemoteCapacityWhileRunningAndPaused(t *testing.T) {
	var maximum atomic.Int64
	maximum.Store(3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		max := int(maximum.Load())
		_ = json.NewEncoder(w).Encode(serverclient.WorkerSettings{MaxActiveJobs: &max})
	}))
	defer server.Close()
	executor := testAdmissionWorker(2)
	executor.activeJobs = 4
	client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "gpu")
	coordinator := NewServerCoordinator(client, executor, nil, testConsoleLogger{})
	for _, max := range []int{3, 1, 0, 4} {
		maximum.Store(int64(max))
		if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
			t.Fatal(err)
		}
		status := executor.AdmissionStatus()
		if status.MaxActiveJobs != max || status.ActiveJobs != 4 {
			t.Fatalf("sync capacity %d = %+v, want limit and uninterrupted accepted jobs", max, status)
		}
		wantEncode := max
		if max == 0 {
			wantEncode = 1
		}
		if status.MaxEncodes != wantEncode {
			t.Errorf("encode target = %d, want %d", status.MaxEncodes, wantEncode)
		}
	}
}

func TestWorkerTelemetryAndPing(t *testing.T) {
	var ping model.PingEventType
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope model.EnvelopEvent
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
		}
		ping = model.PingEventType{}
		if err := json.Unmarshal(envelope.EventData, &ping); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	executor := testAdmissionWorker(2)
	executor.workerConfig.EncodeConfig = &config.FFMPEGConfig{VideoCodec: config.AV1NVENC}
	client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "gpu")
	if err := client.PublishPingEvent(executor.RuntimeTelemetry()); err != nil {
		t.Fatal(err)
	}
	if ping.Runtime == nil || ping.Runtime.Kind != "gpu" || ping.Runtime.Encoder != config.AV1NVENC || ping.Runtime.MaxEncodes != 2 {
		t.Errorf("Ping runtime = %+v", ping.Runtime)
	}
	if err := client.PublishPingEvent(); err != nil {
		t.Fatal(err)
	}
	if ping.Runtime != nil {
		t.Error("legacy Ping unexpectedly includes runtime")
	}
}

func TestRemoteSettingsFailureLeavesAcceptedWorkAndLimitsUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	executor := testAdmissionWorker(2)
	executor.activeJobs = 3
	coordinator := NewServerCoordinator(serverclient.NewServerClient(&web.Config{Domain: server.URL}, "gpu"), executor, nil, testConsoleLogger{})
	if err := coordinator.syncWorkerSettings(t.Context()); err == nil {
		t.Fatal("syncWorkerSettings() succeeded while unavailable")
	}
	status := executor.AdmissionStatus()
	if status.MaxActiveJobs != 2 || status.MaxEncodes != 2 || status.ActiveJobs != 3 {
		t.Errorf("status after failed synchronization = %+v, want unchanged limits and jobs", status)
	}
}

func TestRemoteSynchronizationSerializesWithLocalControl(t *testing.T) {
	var remote atomic.Int64
	remote.Store(3)
	started, release := make(chan struct{}), make(chan struct{})
	var first sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var input serverclient.WorkerSettings
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				return
			}
			remote.Store(int64(*input.MaxActiveJobs))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		max := int(remote.Load())
		first.Do(func() { close(started); <-release })
		_ = json.NewEncoder(w).Encode(serverclient.WorkerSettings{MaxActiveJobs: &max})
	}))
	defer server.Close()
	executor := testAdmissionWorker(2)
	executor.client = serverclient.NewServerClient(&web.Config{Domain: server.URL}, "gpu")
	coordinator := NewServerCoordinator(executor.client, executor, nil, testConsoleLogger{})
	polled, saved := make(chan error, 1), make(chan error, 1)
	go func() { polled <- coordinator.syncWorkerSettings(t.Context()) }()
	<-started
	go func() { saved <- executor.PersistMaxActiveJobs(7) }()
	close(release)
	if err := <-polled; err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status := executor.AdmissionStatus(); status.MaxActiveJobs != 7 || status.MaxEncodes != 7 || remote.Load() != 7 {
		t.Errorf("serialized local/remote control = %+v, persisted %d; want 7", status, remote.Load())
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
	if err := executor.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Errorf("local drain sent %d persistence requests, want zero", requests)
	}
}
