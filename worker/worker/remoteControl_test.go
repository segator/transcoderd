package worker

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/worker/serverclient"
)

func TestRemoteControlsUseOwnedProcessesAndPreserveHolds(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	handler := web.NewWebServer(&web.Config{Token: "worker", DashboardToken: "dashboard"}, sch, nil)
	var failAck atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/worker/control/ack" && failAck.Swap(false) {
			http.Error(w, "retry acknowledgement", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	executor, path := runningRAMWorker(t)
	executor.client = serverclient.NewServerClient(&web.Config{Domain: server.URL, Token: "worker"}, "remote-node")
	coordinator := NewServerCoordinator(executor.client, executor, nil, testConsoleLogger{})
	publish := func() {
		t.Helper()
		if err := executor.client.PublishPingEvent(executor.RuntimeTelemetry()); err != nil {
			t.Fatal(err)
		}
	}
	publish()
	issue := func(action string, ids ...string) *model.WorkerControl {
		t.Helper()
		jobID := ""
		if len(ids) > 0 {
			jobID = ids[0]
		}
		body, _ := json.Marshal(map[string]string{"action": action, "jobId": jobID})
		req, _ := http.NewRequest("POST", server.URL+"/dashboard/api/workers/remote-node/control", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer dashboard")
		req.Header.Set("X-Dashboard-Request", "1")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ = io.ReadAll(response.Body)
		if response.StatusCode != 202 {
			t.Fatalf("control %s = %d %s", action, response.StatusCode, body)
		}
		var c model.WorkerControl
		if err := json.Unmarshal(body, &c); err != nil {
			t.Fatal(err)
		}
		return &c
	}
	apply := func(action string) {
		t.Helper()
		issue(action)
		if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
			t.Fatal(err)
		}
		c, err := repo.GetWorkerControl(t.Context(), "remote-node")
		if err != nil || c.State != "applied" {
			t.Fatalf("ack = %+v %v", c, err)
		}
	}
	apply("stop-after-current")
	status := executor.AdmissionStatus()
	if status.MaxActiveJobs != 0 || status.RAMPaused || !status.Paused || status.SuspendedEncodes != 0 {
		t.Fatalf("queue stop = %+v", status)
	}
	before := ramProgressSize(path)
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	apply("resume-queue")
	if executor.AdmissionStatus().MaxActiveJobs != 2 {
		t.Fatalf("queue restore = %+v", executor.AdmissionStatus())
	}
	apply("pause-encodes")
	status = executor.AdmissionStatus()
	if !status.RAMPaused || status.MaxActiveJobs != 2 || status.SuspendedEncodes != 1 {
		t.Fatalf("RAM pause = %+v", status)
	}
	before = ramProgressSize(path)
	time.Sleep(80 * time.Millisecond)
	if ramProgressSize(path) != before {
		t.Fatal("remote RAM pause did not stop owned process")
	}
	apply("stop-after-current")
	apply("resume-queue")
	if s := executor.AdmissionStatus(); !s.RAMPaused || s.MaxActiveJobs != 2 || s.SuspendedEncodes != 1 {
		t.Fatalf("queue resume cleared RAM hold: %+v", s)
	}
	apply("stop-after-current")
	apply("resume-encodes")
	if s := executor.AdmissionStatus(); s.RAMPaused || s.MaxActiveJobs != 0 || !s.Paused {
		t.Fatalf("RAM resume reopened queue: %+v", s)
	}
	apply("resume-queue")
	// A failed ack must not replay pause over a later local Resume.
	issue("pause-encodes")
	failAck.Store(true)
	if err := coordinator.syncWorkerSettings(t.Context()); err == nil {
		t.Fatal("missing acknowledgement error")
	}
	if err := executor.SetRAMPaused(false); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if executor.AdmissionStatus().RAMPaused {
		t.Fatal("ack retry replayed remote pause over local Resume")
	}
	// Targeting one encode must leave another owned child running.
	firstID := executor.AdmissionStatus().Jobs[0].JobID
	_, secondPath := startRAMWorkerEncode(t, executor)
	publish()
	issue("pause-job", firstID)
	if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
		t.Fatal(err)
	}
	before = ramProgressSize(path)
	otherBefore := ramProgressSize(secondPath)
	awaitRAMWorker(t, func() bool { return ramProgressSize(secondPath) > otherBefore })
	if ramProgressSize(path) != before {
		t.Fatal("individual remote pause did not suspend selected process")
	}
	apply("pause-encodes")
	issue("resume-job", firstID)
	if err := coordinator.syncWorkerSettings(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !executor.AdmissionStatus().RAMPaused || executor.AdmissionStatus().SuspendedEncodes != 2 {
		t.Fatal("individual resume cleared general RAM hold")
	}
	apply("resume-encodes")
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	// A real process exits between acceptance and command application.
	completedID, _, stop := startRAMWorkerEncodeWithCancel(t, executor)
	publish()
	issue("pause-job", completedID)
	stop()
	awaitRAMWorker(t, func() bool { _, present := executor.encodeProcesses.PauseStates()[completedID]; return !present })
	executor.jobs.remove(completedID)

	if err := coordinator.syncWorkerSettings(t.Context()); err == nil {
		t.Fatal("stale job control succeeded")
	}
	result, _ := repo.GetWorkerControl(t.Context(), "remote-node")
	if result.State != "failed" {
		t.Fatalf("stale control=%+v", result)
	}

	// Independent job pauses survive general remote controls.
	id := executor.AdmissionStatus().Jobs[0].JobID
	if err := executor.SetJobPaused(id, true); err != nil {
		t.Fatal(err)
	}
	apply("pause-encodes")
	apply("resume-encodes")
	if executor.AdmissionStatus().SuspendedEncodes != 1 {
		t.Fatal("remote Resume cleared individual hold")
	}
	if err := executor.SetJobPaused(id, false); err != nil {
		t.Fatal(err)
	}
	// Offline/legacy/maintenance workers cannot receive controls.
	if err := executor.DrainAcceptedJobs(); err != nil {
		t.Fatal(err)
	}
	publish()
	if _, err := sch.ControlWorker(t.Context(), "remote-node", "pause-encodes"); err == nil {
		t.Fatal("maintenance control accepted")
	}
}
