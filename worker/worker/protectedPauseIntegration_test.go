package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"

	"github.com/google/uuid"
)

func TestProtectedWorkerParksPrefetchAcrossRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test uses a shell ffprobe fixture")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '{\"format\":{\"duration\":\"1\"},\"streams\":[{\"index\":0,\"codec_type\":\"video\",\"codec_name\":\"h264\",\"avg_frame_rate\":\"24/1\",\"width\":16,\"height\":16}]}'\n"
	if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	repo := repository.NewInMemoryRepository()
	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("film-%d.mkv", i)
		if err := os.WriteFile(filepath.Join(root, name), bytes.Repeat([]byte{byte('a' + i)}, 128), 0o600); err != nil {
			t.Fatal(err)
		}
		video := &model.Job{Id: uuid.New(), SourcePath: name, TargetPath: fmt.Sprintf("target-%d.mkv", i)}
		if err := repo.AddJob(t.Context(), video); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddNewTaskEvent(t.Context(), video.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
			t.Fatal(err)
		}
	}
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root}, repo)
	if err != nil {
		t.Fatal(err)
	}
	credential := "individual-worker-secret"
	digest := sha256.Sum256([]byte(credential))
	serverConfig := &web.Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))}, ActiveAssignmentKey: "v1"}
	handler, err := web.NewProtectedHandler(serverConfig, sch)
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
	temp := t.TempDir()
	workspaceID := uuid.New()
	started := make(chan uuid.UUID, 2)
	release := make(chan struct{}, 2)
	newExecutor := func(maxJobs int) (*JobExecutor, *job.WorkspaceLock) {
		t.Helper()
		cfg := &config.Config{Name: "node", TemporalPath: temp, MaxActiveJobs: maxJobs, EncodeConfig: &config.FFMPEGConfig{VideoCodec: config.HEVCX265}, PGSConfig: &config.PGSConfig{ParallelJobs: 1}}
		e := NewEncodeWorker(cfg, base, console.NewRenderService())
		workspace, err := job.AcquireWorkspaceLock(e.tempPath)
		if err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(e.tempPath, ".workspace-id")
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			if err := os.WriteFile(marker, []byte(workspaceID.String()), 0o600); err != nil {
				t.Fatal(err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		if err := e.ConfigureProtectedMode(t.Context(), client, workspace, workspaceID); err != nil {
			t.Fatal(err)
		}
		encode := step.NewStepExecutor(model.FFMPEGSNotification, func(j *job.Context) []step.Action {
			return []step.Action{{Id: j.JobId.String(), Execute: func(ctx context.Context, _ step.Tracker) error {
				started <- j.JobId
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-release:
				}
				data := []byte("encoded-" + j.JobId.String())
				path := filepath.Join(j.WorkingDir, "output.mkv")
				if err := os.WriteFile(path, data, 0o600); err != nil {
					return err
				}
				sum := sha256.Sum256(data)
				j.Target = &job.VideoData{FilePath: path, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}
				return nil
			}}}
		}, step.WithOnCompleteOpt(func(j *job.Context) { e.stepExecutors[model.UploadNotification].AddJob(j) }), step.WithOnErrorOpt(e.finishProtectedFailure))
		e.trackPipeline(map[model.NotificationType]*step.Executor{model.FFMPEGSNotification: encode})
		e.stepExecutors[model.FFMPEGSNotification] = encode
		return e, workspace
	}
	first, workspace := newExecutor(1)
	if err := first.reconcileProtectedInventory(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	first.Run(&wg, ctx)
	defer func() { cancel(); wg.Wait() }()
	request := func() *job.Context {
		t.Helper()
		if !first.ReserveJobSlot() {
			t.Fatal("worker did not reserve a pipeline slot")
		}
		j, err := requestProtectedJob(ctx, workspace, client, first.pinAdmissionProfile)
		if err != nil || j == nil {
			t.Fatalf("request = %+v, %v", j, err)
		}
		if err := first.startProtectedReservedJob(ctx, j); err != nil {
			t.Fatal(err)
		}
		return j
	}
	j1 := request()
	select {
	case id := <-started:
		if id != j1.JobId {
			t.Fatalf("first encode = %s, want %s", id, j1.JobId)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first encode did not start")
	}
	j2 := request()
	waitProtectedState(t, first, j2.JobId, "queued")
	if err := first.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	first.BeginRestartBarrier()
	waitProtectedState(t, first, j2.JobId, "parked")
	if status := first.AdmissionStatus(); status.SafeIdle == nil || *status.SafeIdle {
		t.Fatalf("active encode was reported idle: %+v", status)
	}
	release <- struct{}{}
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	if err := first.WaitSafeIdle(waitCtx); err != nil {
		t.Fatalf("wait for active encode and upload: %v", err)
	}
	assertNoFFmpegStarted(t, repo, j2.JobId)
	cancel()
	wg.Wait()
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}

	second, workspace := newExecutor(0)
	defer func() { _ = workspace.Close() }()
	ctx2, cancel2 := context.WithCancel(t.Context())
	var wg2 sync.WaitGroup
	second.Run(&wg2, ctx2)
	defer func() { cancel2(); wg2.Wait() }()
	coordinator := NewServerCoordinator(base, second, nil, console.NewRenderService().Logger())
	if len(second.parkedRecoveries) != 1 || !coordinator.resolveParkedProtectedAssignment(ctx2, second.parkedRecoveries[0]) {
		t.Fatal("parked assignment was not reconciled after restart")
	}
	waitProtectedState(t, second, j2.JobId, "parked")
	if err := second.reconcileProtectedInventory(ctx2); err != nil {
		t.Fatalf("reconcile recovered assignment: %v", err)
	}
	assertNoFFmpegStarted(t, repo, j2.JobId)
	if err := second.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-started:
		if id != j2.JobId {
			t.Fatalf("resumed encode = %s, want %s", id, j2.JobId)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prefetched encode did not resume")
	}
	release <- struct{}{}
	deadline := time.Now().Add(5 * time.Second)
	for second.ActiveJobs() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	stored, err := repo.GetJob(t.Context(), j2.JobId.String())
	if err != nil || stored.Events.GetStatus() != model.CompletedNotificationStatus || second.ActiveJobs() != 0 {
		t.Fatalf("resumed job = %+v, %v; active=%d", stored, err, second.ActiveJobs())
	}
	downloadStarts := 0
	for _, event := range stored.Events {
		if event.NotificationType == model.DownloadNotification && event.Status == model.StartedNotificationStatus {
			downloadStarts++
		}
	}
	if downloadStarts != 1 {
		t.Fatalf("download started %d times, want once", downloadStarts)
	}
}

func waitProtectedState(t *testing.T, e *JobExecutor, id uuid.UUID, state string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := e.jobs.get(id.String())
		if ok && status.Phase == string(model.FFMPEGSNotification) && status.State == state {
			return
		}
		if errorText := e.AdmissionStatus().RecoveryError; errorText != "" {
			t.Fatalf("worker recovery error while waiting for %s: %s", state, errorText)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s: %+v", id, state, e.AdmissionStatus())
}

func assertNoFFmpegStarted(t *testing.T, repo repository.Repository, id uuid.UUID) {
	t.Helper()
	stored, err := repo.GetJob(t.Context(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range stored.Events {
		if event.NotificationType == model.FFMPEGSNotification && event.Status == model.StartedNotificationStatus {
			t.Fatal("prefetched job started encoding while the queue was stopped")
		}
	}
}
