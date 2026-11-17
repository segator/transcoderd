package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	"github.com/google/uuid"
)

func TestProtectedExecutorRunsRealFFmpeg(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not available")
	}
	encoders, err := exec.CommandContext(t.Context(), ffmpegPath, "-hide_banner", "-encoders").CombinedOutput()
	if err != nil || !strings.Contains(string(encoders), "libx265") {
		t.Skip("libx265 is not available")
	}
	root := t.TempDir()
	source := filepath.Join(root, "film.mkv")
	generate := exec.CommandContext(t.Context(), ffmpegPath, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=12", "-t", "2", "-c:v", "ffv1", "-threads", "1", "-y", source)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Skipf("cannot generate test source: %v (%s)", err, output)
	}
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root}, repo)
	if err != nil {
		t.Fatal(err)
	}
	video := &model.Job{Id: uuid.New(), SourcePath: "film.mkv", TargetPath: "target.mkv"}
	if err := repo.AddJob(t.Context(), video); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(t.Context(), video.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
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
	workerConfig := &config.Config{Name: "node", TemporalPath: t.TempDir(), MaxActiveJobs: 1, VerifyDeltaTime: 10, EncodeConfig: &config.FFMPEGConfig{VideoCodec: config.HEVCX265, VideoPreset: "ultrafast", VideoCRF: 35, Threads: 1}, PGSConfig: &config.PGSConfig{ParallelJobs: 1}}
	executor := NewEncodeWorker(workerConfig, base, console.NewRenderService())
	if err := executor.configureProtectedClient(client); err != nil {
		t.Fatal(err)
	}
	workspace, err := job.AcquireWorkspaceLock(executor.tempPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	executor.Run(&wg, ctx)
	defer func() { cancel(); wg.Wait() }()
	if !executor.ReserveJobSlot() {
		t.Fatal("worker did not reserve an admission slot")
	}
	j, err := requestProtectedJob(ctx, workspace, client, executor.pinAdmissionProfile)
	if err != nil || j == nil {
		t.Fatalf("request protected job = %+v, %v", j, err)
	}
	if err := executor.startProtectedReservedJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		stored, err := repo.GetJob(ctx, video.Id.String())
		if err != nil {
			t.Fatal(err)
		}
		status := stored.Events.GetStatus()
		if status == model.FailedNotificationStatus || executor.AdmissionStatus().RecoveryError != "" {
			t.Fatalf("protected encode failed: status=%s recovery=%s events=%+v", status, executor.AdmissionStatus().RecoveryError, stored.Events)
		}
		if status == model.CompletedNotificationStatus && executor.ActiveJobs() == 0 {
			if _, err := os.Stat(filepath.Join(root, stored.TargetPath)); err != nil {
				t.Fatalf("completed output is unavailable: %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("protected encode did not finish: status=%s events=%+v", status, stored.Events)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
