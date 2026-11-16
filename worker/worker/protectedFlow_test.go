package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

func TestProtectedFlowStopsRestartsAndResumesPrefetch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this protocol test uses a shell ffprobe fixture")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '{\"format\":{\"duration\":\"1\"},\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"av1\",\"width\":16,\"height\":16}]}'\n"
	if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := t.Context()
	root := t.TempDir()
	source := []byte("source movie for prefetch")
	if err := os.WriteFile(filepath.Join(root, "film.mkv"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: root}, repo)
	if err != nil {
		t.Fatal(err)
	}
	video := &model.Job{Id: uuid.New(), SourcePath: "film.mkv", TargetPath: "target.mkv"}
	if err := repo.AddJob(ctx, video); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, video.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	credential := "individual-worker-secret"
	digest := sha256.Sum256([]byte(credential))
	config := &web.Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32))}, ActiveAssignmentKey: "v1"}
	handler, err := web.NewProtectedHandler(config, sch)
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
	workspacePath := t.TempDir()
	workspace, err := job.AcquireWorkspaceLock(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	j, err := requestProtectedJob(ctx, workspace, client, func(*job.Context) error { return nil })
	if err != nil || j == nil || j.JobId != video.Id {
		t.Fatalf("protected request = %+v, %v", j, err)
	}
	claim := *j.Assignment
	claim.InstanceID = uuid.New()
	guard, err := startLeaseGuard(ctx, func(ctx context.Context) (time.Time, error) { return client.AcquireLease(ctx, claim) }, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err := j.RecordAcquiredLease(claim.InstanceID); err != nil {
		t.Fatal(err)
	}
	publisher := &protectedEventPublisher{client: client, workerName: "node", retryDelay: time.Millisecond}
	for _, event := range []struct {
		phase  model.NotificationType
		status model.NotificationStatus
	}{{model.JobNotification, model.StartedNotificationStatus}, {model.DownloadNotification, model.StartedNotificationStatus}} {
		if err := publisher.Publish(guard.ctx, j, event.phase, event.status, ""); err != nil {
			t.Fatal(err)
		}
	}
	download, err := client.Download(guard.ctx, *j.Assignment)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if err != nil || !bytes.Equal(downloaded, source) {
		t.Fatalf("download = %q, %v", downloaded, err)
	}
	checksum, err := client.Checksum(guard.ctx, *j.Assignment)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(j.WorkingDir, "source.mkv")
	if err := os.WriteFile(path, downloaded, 0o600); err != nil {
		t.Fatal(err)
	}
	j.Source = &job.VideoData{FilePath: path, Size: int64(len(downloaded)), Checksum: checksum, FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{}}}
	if err := publisher.Publish(guard.ctx, j, model.DownloadNotification, model.CompletedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	j.NextPhase = model.FFMPEGSNotification
	if err := j.PersistJobContext(); err != nil {
		t.Fatal(err)
	}
	if err := parkProtectedJob(ctx, j, guard, client, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetJob(ctx, video.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	jobStarted := current.Events.GetLatestPerNotificationType(model.JobNotification)
	if requeued, err := repo.RequeueTimedOutJob(ctx, video.Id.String(), jobStarted.EventID, time.Now().Add(time.Hour)); err != nil || requeued {
		t.Fatalf("parked timeout = %t, %v", requeued, err)
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	workspace, err = job.AcquireWorkspaceLock(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	j, err = job.ReadContextFromDiskByPath(filepath.Join(filepath.Dir(path), video.Id.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if resumedGuard, err := recoverParkedJob(ctx, j, "node", client, true, time.Millisecond); err != nil || resumedGuard != nil || !j.Parked {
		t.Fatalf("recovery while stopped = %v, %v; parked=%t", resumedGuard, err, j.Parked)
	}
	guard, err = recoverParkedJob(ctx, j, "node", client, false, time.Millisecond)
	if err != nil || guard == nil || j.Parked || j.NextPhase != model.FFMPEGSNotification || j.Source.FilePath != path {
		t.Fatalf("resume recovered job = %v, %v", guard, err)
	}
	defer guard.Close()
	for _, event := range []struct {
		phase  model.NotificationType
		status model.NotificationStatus
	}{{model.FFMPEGSNotification, model.StartedNotificationStatus}, {model.FFMPEGSNotification, model.CompletedNotificationStatus}, {model.UploadNotification, model.StartedNotificationStatus}} {
		if err := publisher.Publish(guard.ctx, j, event.phase, event.status, ""); err != nil {
			t.Fatal(err)
		}
	}
	encoded := []byte("encoded movie")
	encodedSum := sha256.Sum256(encoded)
	if err := client.Upload(guard.ctx, *j.Assignment, bytes.NewReader(encoded), int64(len(encoded)), hex.EncodeToString(encodedSum[:])); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(guard.ctx, j, model.UploadNotification, model.CompletedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(guard.ctx, j, model.JobNotification, model.CompletedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	current, err = repo.GetJob(ctx, video.Id.String())
	if err != nil || current.Events.GetStatus() != model.CompletedNotificationStatus {
		t.Fatalf("completed job = %+v, %v", current, err)
	}
	output, err := os.ReadFile(filepath.Join(root, current.TargetPath))
	if err != nil || !bytes.Equal(output, encoded) {
		t.Fatalf("published output = %q, %v", output, err)
	}
}
