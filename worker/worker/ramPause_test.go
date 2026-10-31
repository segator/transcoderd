package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

func TestRAMWorkerChild(t *testing.T) {
	path := os.Getenv("TRANSCODERD_RAM_WORKER_CHILD")
	if path == "" {
		return
	}
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(2)
		}
		_, _ = f.Write([]byte("x"))
		_ = f.Close()
		time.Sleep(10 * time.Millisecond)
	}
}

func ramProgressSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
func awaitRAMWorker(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("RAM worker condition timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func runningRAMWorker(t *testing.T) (*JobExecutor, string) {
	t.Helper()
	worker := testAdmissionWorker(2)
	worker.encodeProcesses = &command.PauseController{}
	worker.pauseMode = config.PauseRAM
	_, path := startRAMWorkerEncode(t, worker)
	return worker, path
}

func startRAMWorkerEncode(t *testing.T, worker *JobExecutor) (string, string) {
	t.Helper()
	id := uuid.New()
	worker.jobs.set(&job.Context{JobId: id}, model.FFMPEGSNotification, "running")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "progress")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := command.NewCommand(exe, "-test.run=^TestRAMWorkerChild$").AddEnv("TRANSCODERD_RAM_WORKER_CHILD="+path).SetPauseController(worker.encodeProcesses, id.String()).RunWithContext(ctx)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("suspended child did not terminate")
		}
	})
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > 2 })
	return id.String(), path
}

func TestRAMWorkerPauseModeChangesAndResume(t *testing.T) {
	worker, path := runningRAMWorker(t)
	if err := worker.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	status := worker.AdmissionStatus()
	if status.SuspendedEncodes != 1 || status.PauseMode != "ram" || len(status.Jobs) != 1 || status.Jobs[0].State != "paused" || status.Accepting {
		t.Fatalf("paused status = %+v", status)
	}
	before := ramProgressSize(path)
	time.Sleep(80 * time.Millisecond)
	if got := ramProgressSize(path); got != before {
		t.Fatalf("paused progress=%d,want %d", got, before)
	}
	if err := worker.SetPauseMode("finish"); err != nil {
		t.Fatal(err)
	}
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	if !worker.AdmissionStatus().Paused || worker.ReserveJobSlot() {
		t.Fatal("finish mode reopened the queue or admission")
	}
	if err := worker.SetPauseMode("chunked"); err == nil {
		t.Fatal("unimplemented mode accepted")
	}
	if err := worker.SetPauseMode("ram"); err != nil {
		t.Fatal(err)
	}
	if worker.AdmissionStatus().SuspendedEncodes != 1 {
		t.Fatal("live RAM mode did not suspend encode")
	}
	if err := worker.SetMaxActiveJobs(2); err != nil {
		t.Fatal(err)
	}
	before = ramProgressSize(path)
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	if worker.AdmissionStatus().SuspendedEncodes != 0 || worker.AdmissionStatus().Paused {
		t.Fatal("resume retained suspension")
	}
}

func TestRAMWorkerMaintenanceResumesSuspendedEncodes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		drain func(*JobExecutor) error
	}{{"configuration", (*JobExecutor).DrainAcceptedJobs}, {"update", (*JobExecutor).HoldAdmission}} {
		t.Run(tt.name, func(t *testing.T) {
			worker, path := runningRAMWorker(t)
			if err := worker.SetMaxActiveJobs(0); err != nil {
				t.Fatal(err)
			}
			before := ramProgressSize(path)
			if err := tt.drain(worker); err != nil {
				t.Fatal(err)
			}
			awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
			if status := worker.AdmissionStatus(); status.Paused || status.SuspendedEncodes != 0 || status.Accepting {
				t.Fatalf("maintenance status=%+v", status)
			}
			if worker.ReserveJobSlot() {
				t.Fatal("maintenance reopened assignments")
			}
		})
	}
}

func TestIndividualWorkerPausePreservesOtherJobAndSlots(t *testing.T) {
	worker, first := runningRAMWorker(t)
	firstID := worker.AdmissionStatus().Jobs[0].JobID
	secondID, second := startRAMWorkerEncode(t, worker)
	for range 2 {
		if !worker.encodePool.tryStart() {
			t.Fatal("failed to reserve encode slots")
		}
	}
	if err := worker.SetPauseMode(config.PauseFinish); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetJobPaused(firstID, true); err != nil {
		t.Fatal(err)
	}
	a, b := ramProgressSize(first), ramProgressSize(second)
	time.Sleep(80 * time.Millisecond)
	if ramProgressSize(first) != a || ramProgressSize(second) <= b {
		t.Fatal("pausing one job affected another encode")
	}
	status := worker.AdmissionStatus()
	if status.MaxActiveJobs != 2 || status.ActiveEncodes != 2 || status.SuspendedEncodes != 1 || worker.encodePool.tryStart() {
		t.Fatalf("individual pause changed limits/slots: %+v", status)
	}
	if err := worker.SetPauseMode(config.PauseRAM); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetMaxActiveJobs(2); err != nil {
		t.Fatal(err)
	}
	if ramProgressSize(first) != a {
		t.Fatal("general Resume resumed an individually paused job")
	}
	if err := worker.SetJobPaused(secondID, true); err != nil {
		t.Fatal(err)
	}
	if err := worker.DrainAcceptedJobs(); err != nil {
		t.Fatal(err)
	}
	awaitRAMWorker(t, func() bool { return ramProgressSize(first) > a })
	if err := worker.SetJobPaused(firstID, true); err == nil {
		t.Fatal("individual pause interrupted maintenance drain")
	}
	if worker.AdmissionStatus().SuspendedEncodes != 0 || worker.ReserveJobSlot() {
		t.Fatal("maintenance retained holds or reopened admission")
	}
	if err := worker.SetJobPaused(uuid.NewString(), true); err == nil {
		t.Fatal("stale job accepted")
	}
}

func TestSeparateQueueAndRAMPauseOrders(t *testing.T) {
	worker, path := runningRAMWorker(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	worker.client = serverclient.NewServerClient(&web.Config{Domain: server.URL}, "separate")
	if err := worker.PersistAdmissionLimit(0); err != nil {
		t.Fatal(err)
	}
	before := ramProgressSize(path)
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	if worker.AdmissionStatus().RAMPaused || worker.ReserveJobSlot() || !worker.encodePool.Paused() {
		t.Fatal("queue stop suspended active work or left admission open")
	}
	if err := worker.SetRAMPaused(true); err != nil {
		t.Fatal(err)
	}
	before = ramProgressSize(path)
	time.Sleep(80 * time.Millisecond)
	if ramProgressSize(path) != before || worker.AdmissionStatus().MaxActiveJobs != 0 {
		t.Fatal("RAM pause did not preserve queue stop")
	}
	if err := worker.PersistAdmissionLimit(2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if ramProgressSize(path) != before || !worker.AdmissionStatus().RAMPaused || worker.ReserveJobSlot() {
		t.Fatal("queue Resume bypassed RAM hold")
	}
	if err := worker.SetRAMPaused(false); err != nil {
		t.Fatal(err)
	}
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	if worker.encodePool.Paused() || !worker.AdmissionStatus().Accepting {
		t.Fatal("independent holds did not release")
	}
	if err := worker.PersistAdmissionLimit(0); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetRAMPaused(true); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetRAMPaused(false); err != nil {
		t.Fatal(err)
	}
	before = ramProgressSize(path)
	awaitRAMWorker(t, func() bool { return ramProgressSize(path) > before })
	if worker.ReserveJobSlot() || !worker.encodePool.Paused() || worker.AdmissionStatus().MaxActiveJobs != 0 {
		t.Fatal("RAM Resume reopened a stopped queue")
	}
	id := worker.AdmissionStatus().Jobs[0].JobID
	if err := worker.SetJobPaused(id, true); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetRAMPaused(true); err != nil {
		t.Fatal(err)
	}
	if err := worker.SetRAMPaused(false); err != nil {
		t.Fatal(err)
	}
	if worker.AdmissionStatus().SuspendedEncodes != 1 {
		t.Fatal("RAM Resume cleared an individual hold")
	}
	if err := worker.HoldAdmission(); err != nil {
		t.Fatal(err)
	}
	if worker.AdmissionStatus().RAMPaused || worker.AdmissionStatus().SuspendedEncodes != 0 {
		t.Fatal("maintenance retained RAM suspension")
	}
	if err := worker.SetRAMPaused(true); err == nil {
		t.Fatal("RAM pause interrupted maintenance")
	}
}

func TestSeparateRAMStartupWaitsForResume(t *testing.T) {
	client := serverclient.NewServerClient(&web.Config{Domain: "http://127.0.0.1"}, "ram-start")
	worker := NewEncodeWorker(&config.Config{Name: "ram-start", TemporalPath: t.TempDir(), MaxActiveJobs: 2, RAMPaused: true, EncodeConfig: &config.FFMPEGConfig{}, PGSConfig: &config.PGSConfig{}}, client, nil)
	worker.RequireWorkerSettings()
	if err := worker.SetMaxActiveJobs(3); err != nil {
		t.Fatal(err)
	}
	worker.MarkWorkerSettingsResolved()
	if !worker.AdmissionStatus().RAMPaused || worker.ReserveJobSlot() || !worker.encodePool.Paused() {
		t.Fatal("settings resolution released startup RAM hold")
	}
	if err := worker.SetRAMPaused(false); err != nil {
		t.Fatal(err)
	}
	if worker.encodePool.Paused() || !worker.ReserveJobSlot() {
		t.Fatal("explicit Resume did not release startup RAM hold")
	}
}
