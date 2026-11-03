package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/control"
	localdashboard "transcoder/worker/dashboard"
	"transcoder/worker/serverclient"
)

func TestWorkerUIAndFleetDashboardSynchronizeBothDirections(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryRepository()
	scheduler, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	fleet := httptest.NewServer(web.NewWebServer(&web.Config{Token: "worker-api", DashboardToken: "encoding123"}, scheduler, nil))
	defer fleet.Close()
	executor := testAdmissionWorker(2)
	executor.client = serverclient.NewServerClient(&web.Config{Domain: fleet.URL, Token: "worker-api"}, "synced-node")
	coordinator := NewServerCoordinator(executor.client, executor, nil, testConsoleLogger{})
	local, err := localdashboard.Start("127.0.0.1:0", "local-secret", executor)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Shutdown() }()
	go func() { _ = local.Serve() }()
	localURL := "http://" + local.Addr().String()
	call := func(method, url, token, body string, want int) []byte {
		t.Helper()
		r, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Dashboard-Request", "1")
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s = %d %s, want %d", method, url, response.StatusCode, raw, want)
		}
		return raw
	}
	publish := func() model.Dashboard {
		t.Helper()
		if err := executor.client.PublishPingEvent(executor.RuntimeTelemetry()); err != nil {
			t.Fatal(err)
		}
		var d model.Dashboard
		if err := json.Unmarshal(call("GET", fleet.URL+"/dashboard/api/stats", "encoding123", "", 200), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	call("POST", localURL+"/api/worker/max-jobs", "local-secret", `{"cpuJobs":2,"gpuJobs":3}`, 200)
	stored, err := repo.GetWorkerSettings(ctx, "synced-node")
	if err != nil || !stored.Independent() || *stored.CPUJobs != 2 || *stored.GPUJobs != 3 {
		t.Fatalf("local UI persistence = %+v %v", stored, err)
	}
	d := publish()
	if len(d.Workers) != 1 || d.Workers[0].Runtime.CPUJobs != 2 || d.Workers[0].Runtime.GPUJobs != 3 {
		t.Fatalf("fleet observed pair = %+v", d.Workers)
	}
	call("POST", localURL+"/api/worker/encodes/pause", "local-secret", "{}", 200)
	if err := coordinator.syncWorkerSettings(ctx); err != nil {
		t.Fatal(err)
	}
	d = publish()
	if !d.Workers[0].Runtime.RAMPaused || !d.Workers[0].Runtime.Paused || d.Workers[0].Runtime.CPUJobs != 2 || d.Workers[0].Runtime.GPUJobs != 3 {
		t.Fatalf("RAM hold changed pair or missing telemetry: %+v", d.Workers[0].Runtime)
	}
	call("PUT", fleet.URL+"/dashboard/api/workers/synced-node/settings", "encoding123", `{"cpuJobs":4,"gpuJobs":0}`, 204)
	if err := coordinator.syncWorkerSettings(ctx); err != nil {
		t.Fatal(err)
	}
	var status struct{ Worker control.AdmissionStatus }
	if err := json.Unmarshal(call("GET", localURL+"/api/status", "local-secret", "", 200), &status); err != nil {
		t.Fatal(err)
	}
	if status.Worker.CPUJobs != 4 || status.Worker.GPUJobs != 0 || !status.Worker.RAMPaused {
		t.Fatalf("fleet -> local UI = %+v", status.Worker)
	}
	call("POST", localURL+"/api/worker/encodes/resume", "local-secret", "{}", 200)
	if s := executor.AdmissionStatus(); s.Paused || !s.Accepting || s.CPUJobs != 4 || s.GPUJobs != 0 {
		t.Fatalf("resume lost independent limits = %+v", s)
	}

	// Exercise a real controllable child, not just fabricated pause flags.
	executor.encodeProcesses = &command.PauseController{}
	pausedID, _ := startRAMWorkerEncode(t, executor)
	parsedID, err := uuid.Parse(pausedID)
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: parsedID, SourcePath: "pause-observation.mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []model.NotificationType{model.JobNotification, model.FFMPEGSNotification} {
		event := job.AddEvent(phase, model.StartedNotificationStatus)
		event.WorkerName = "synced-node"
		if err := repo.AddNewTaskEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	call("POST", localURL+"/api/worker/jobs/"+pausedID+"/pause", "local-secret", "{}", 200)
	d = publish()
	runtime := d.Workers[0].Runtime
	if runtime.SuspendedEncodes != 1 || len(runtime.Jobs) != 1 || runtime.Jobs[0].State != "paused" || !runtime.Jobs[0].IndividuallyPaused {
		t.Fatalf("individual pause telemetry = %+v", runtime)
	}
	call("POST", localURL+"/api/worker/encodes/pause", "local-secret", "{}", 200)
	call("POST", localURL+"/api/worker/encodes/resume", "local-secret", "{}", 200)
	d = publish()
	if d.Workers[0].Runtime.SuspendedEncodes != 1 {
		t.Fatal("general Resume cleared the individual hold")
	}
	call("POST", localURL+"/api/worker/jobs/"+pausedID+"/resume", "local-secret", "{}", 200)
	d = publish()
	if d.Workers[0].Runtime.SuspendedEncodes != 0 || d.Workers[0].Runtime.Jobs[0].State == "paused" {
		t.Fatal("individual Resume did not synchronize")
	}
	call("POST", localURL+"/api/worker/max-jobs", "local-secret", `{"cpuJobs":0,"gpuJobs":0}`, 200)
	if err := coordinator.syncWorkerSettings(ctx); err != nil {
		t.Fatal(err)
	}
	d = publish()
	if !d.Workers[0].Runtime.Paused || d.Workers[0].Runtime.RAMPaused || d.Workers[0].Runtime.Accepting {
		t.Fatalf("queue stop = %+v", d.Workers[0].Runtime)
	}
	if _, ok := executor.encodePool.tryStartEncoder("cpu"); ok {
		t.Error("queue stop started an accepted pending encode")
	}
	if err := executor.DrainAcceptedJobs(); err != nil {
		t.Fatal(err)
	}
	call("PUT", fleet.URL+"/dashboard/api/workers/synced-node/settings", "encoding123", `{"cpuJobs":1,"gpuJobs":1}`, 204)
	if err := coordinator.syncWorkerSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if s := executor.AdmissionStatus(); s.Accepting || !s.Draining || s.Paused {
		t.Fatalf("polling reopened maintenance hold = %+v", s)
	}
	// Windows supervised profiles cannot silently turn into dual-class processes.
	executor.managedLimits = true
	executor.workerConfig.EncodeConfig = &config.FFMPEGConfig{VideoCodec: config.AV1SVT}
	publish()
	call("PUT", fleet.URL+"/dashboard/api/workers/synced-node/settings", "encoding123", `{"cpuJobs":1,"gpuJobs":1}`, 400)
	call("PUT", fleet.URL+"/dashboard/api/workers/synced-node/settings", "encoding123", `{"cpuJobs":65,"gpuJobs":0}`, 400)
	call("PUT", fleet.URL+"/dashboard/api/workers/synced-node/settings", "encoding123", `{"cpuJobs":4,"gpuJobs":0}`, 204)
}

func TestLocalPairWriteFailurePreservesRuntimeAndRAMHold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	executor := testAdmissionWorker(2)
	executor.client = serverclient.NewServerClient(&web.Config{Domain: server.URL}, "node")
	if err := executor.SetEncoderLimits(2, 3); err != nil {
		t.Fatal(err)
	}
	if err := executor.SetRAMPaused(true); err != nil {
		t.Fatal(err)
	}
	if err := executor.PersistEncoderLimits(1, 0); err == nil {
		t.Fatal("failed server write reported success")
	}
	if s := executor.AdmissionStatus(); s.CPUJobs != 2 || s.GPUJobs != 3 || !s.RAMPaused {
		t.Fatalf("failed write changed local state = %+v", s)
	}
}

func TestFinishHoldAndManagedEmptyBootstrap(t *testing.T) {
	executor := testAdmissionWorker(2)
	executor.UseManagedLimits()
	executor.RequireWorkerSettings()
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			writes++
			w.WriteHeader(204)
			return
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "node")
	executor.client = client
	coordinator := NewServerCoordinator(client, executor, nil, testConsoleLogger{})
	if !coordinator.loadPersistedWorkerSettings(t.Context()) || writes != 1 {
		t.Fatalf("empty managed bootstrap writes = %d", writes)
	}
	if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatal("normal poll overwrote settings")
	}
	if err := executor.FinishAcceptedJobs(); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if s := executor.AdmissionStatus(); s.Accepting || !s.Finishing || s.Paused {
		t.Fatalf("finish hold = %+v", s)
	}
}
