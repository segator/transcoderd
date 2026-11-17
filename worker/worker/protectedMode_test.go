package worker

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

func TestProtectedModeRefusesUnknownServerAssignments(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	credential := "node-secret"
	digest := sha256.Sum256([]byte(credential))
	handler, err := web.NewProtectedHandler(&web.Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}, ActiveAssignmentKey: "v1"}, sch)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	base := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "node")
	client, err := base.Protected(credential)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Name: "node", TemporalPath: t.TempDir(), MaxActiveJobs: 1, EncodeConfig: &config.FFMPEGConfig{VideoCodec: config.HEVCX265}, PGSConfig: &config.PGSConfig{ParallelJobs: 1}}
	e := NewEncodeWorker(cfg, base, console.NewRenderService())
	workspace, err := job.AcquireWorkspaceLock(e.tempPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	id := uuid.New()
	if err := os.WriteFile(filepath.Join(e.tempPath, ".workspace-id"), []byte(id.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.ConfigureProtectedMode(t.Context(), client, workspace, id); err != nil {
		t.Fatal(err)
	}
	if e.ReserveJobSlot() || e.AdmissionStatus().SafeIdle != nil {
		t.Fatal("admission or safe idle opened before ownership reconciliation")
	}
	if _, ok := e.encodePool.tryStartEncoder(""); ok {
		t.Fatal("encode started before ownership reconciliation")
	}
	e.BeginRestartBarrier()
	e.ReleaseRestartBarrier()
	if _, ok := e.encodePool.tryStartEncoder(""); ok {
		t.Fatal("releasing the restart barrier bypassed startup reconciliation")
	}
	legacy := &model.Job{Id: uuid.New(), SourcePath: "unknown.mkv"}
	if err := repo.AddJob(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(t.Context(), legacy.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assigned := legacy.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
	assigned.WorkerName = "node"
	if err := repo.AddNewTaskEvent(t.Context(), assigned); err != nil {
		t.Fatal(err)
	}
	if err := e.reconcileProtectedInventory(t.Context()); err == nil || e.AdmissionStatus().SafeIdle != nil || e.ReserveJobSlot() {
		t.Fatalf("unknown assignment was accepted: %v", err)
	}
	if err := repo.CancelJobAssignment(t.Context(), legacy.Id.String()); err != nil {
		t.Fatal(err)
	}
	if err := e.reconcileProtectedInventory(t.Context()); err != nil {
		t.Fatal(err)
	}
	kind, ok := e.encodePool.tryStartEncoder("")
	if !ok {
		t.Fatal("encode remained blocked after ownership reconciliation")
	}
	e.encodePool.finishEncoder(kind)
	e.BeginRestartBarrier()
	if status := e.AdmissionStatus(); status.SafeIdle == nil || !*status.SafeIdle {
		t.Fatalf("reconciled empty worker = %+v", status)
	}
	e.ReleaseRestartBarrier()
	if err := os.Remove(filepath.Join(e.tempPath, ".workspace-id")); err != nil {
		t.Fatal(err)
	}
	if e.ReserveJobSlot() || e.AdmissionStatus().Accepting {
		t.Fatal("admission remained open after the workspace identity disappeared")
	}
}

func TestProtectedAdmissionWaitsForParkedAssignments(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 1}, maxActiveJobs: 1,
		encodePool: newEncodeConcurrency(1), protectedClient: &serverclient.ProtectedClient{}, protectedReconciled: true}
	j := &job.Context{JobId: uuid.New()}
	for _, state := range []string{"parked", "reconciling"} {
		e.jobs.set(j, model.FFMPEGSNotification, state)
		if e.ReserveJobSlot() {
			t.Fatalf("admitted new work while an assignment was %s", state)
		}
	}
	e.jobs.set(j, model.FFMPEGSNotification, "queued")
	e.reclaimedJobs = map[uuid.UUID]bool{j.JobId: true}
	if e.ReserveJobSlot() || e.AdmissionStatus().Accepting {
		t.Fatal("admission reopened before the reclaimed encode started")
	}
	delete(e.reclaimedJobs, j.JobId)
	if !e.ReserveJobSlot() {
		t.Fatal("admission did not reopen after the parked assignment was reclaimed")
	}
	e.ReleaseJobSlot()
}

func TestProtectedStartupRecoversAcceptedAssignmentBeforeDownload(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	video := &model.Job{Id: uuid.New(), SourcePath: "movie.mkv"}
	if err := repo.AddJob(t.Context(), video); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(t.Context(), video.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	secret := "node-secret"
	digest := sha256.Sum256([]byte(secret))
	handler, err := web.NewProtectedHandler(&web.Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}, ActiveAssignmentKey: "v1"}, sch)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	base := serverclient.NewServerClient(&web.Config{Domain: server.URL}, "node")
	client, err := base.Protected(secret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Name: "node", TemporalPath: t.TempDir(), MaxActiveJobs: 1, EncodeConfig: &config.FFMPEGConfig{VideoCodec: config.HEVCX265}, PGSConfig: &config.PGSConfig{ParallelJobs: 1}}
	e := NewEncodeWorker(cfg, base, console.NewRenderService())
	workspace, err := job.AcquireWorkspaceLock(e.tempPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	identity := uuid.New()
	if err := os.WriteFile(filepath.Join(e.tempPath, ".workspace-id"), []byte(identity.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	assigned, err := requestProtectedJob(t.Context(), workspace, client, e.pinAdmissionProfile)
	if err != nil || assigned == nil {
		t.Fatalf("request assignment = %+v, %v", assigned, err)
	}
	if err := e.ConfigureProtectedMode(t.Context(), client, workspace, identity); err != nil {
		t.Fatal(err)
	}
	e.resumeJobsContext(t.Context())
	if e.recoveryError != "" || len(e.initialRecoveries) != 1 || e.reservedJobs != 1 {
		t.Fatalf("initial recovery = %d reserved=%d error=%q", len(e.initialRecoveries), e.reservedJobs, e.recoveryError)
	}
	if err := e.startProtectedReservedJob(t.Context(), e.initialRecoveries[0]); err != nil {
		t.Fatal(err)
	}
	defer e.protectedExecution(assigned.JobId).currentGuard().Close()
	if err := e.reconcileProtectedInventory(t.Context()); err != nil {
		t.Fatalf("reconcile recovered assignment: %v", err)
	}
	if e.ActiveJobs() != 1 || e.reservedJobs != 0 {
		t.Fatalf("recovered job count = %d reserved=%d", e.ActiveJobs(), e.reservedJobs)
	}
}
