//go:build integration

package worker

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

func TestProtectedFailureRecoveryAfterLostPhaseAcknowledgement(t *testing.T) {
	ctx := t.Context()
	repo := repository.NewInMemoryRepository()
	video := &model.Job{Id: uuid.New(), SourcePath: "film.mkv", TargetPath: "target.mkv"}
	if err := repo.AddJob(ctx, video); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, video.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	sch, err := scheduler.NewScheduler(&scheduler.Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	secret := "individual-worker-secret"
	digest := sha256.Sum256([]byte(secret))
	handler, err := web.NewProtectedHandler(&web.Config{ProtectedWorkerHashes: map[string]string{"node": hex.EncodeToString(digest[:])}, AssignmentSigningKeys: map[string]string{"v1": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))}, ActiveAssignmentKey: "v1"}, sch)
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
	workspace, err := job.AcquireWorkspaceLock(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	j, err := requestProtectedJob(ctx, workspace, client, func(*job.Context) error { return nil })
	if err != nil || j == nil {
		t.Fatalf("request = %+v, %v", j, err)
	}
	claim := *j.Assignment
	claim.InstanceID = uuid.New()
	expiry, err := repo.AcquireProtectedLease(ctx, "node", claim, claim.InstanceID, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.RecordAcquiredLease(claim.InstanceID); err != nil {
		t.Fatal(err)
	}
	publisher := &protectedEventPublisher{client: client, workerName: "node", retryDelay: time.Millisecond}
	if err := publisher.Publish(ctx, j, model.JobNotification, model.StartedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	j.NextPhase = model.FFMPEGSNotification
	if err := publisher.Publish(ctx, j, model.FFMPEGSNotification, model.StartedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	if err := j.RecordPhaseFailure(model.FFMPEGSNotification, "encoder failed"); err != nil {
		t.Fatal(err)
	}
	event, err := j.NewProtectedEvent("node", model.FFMPEGSNotification, model.FailedNotificationStatus, "encoder failed")
	if err != nil {
		t.Fatal(err)
	}
	if err := j.StageProtectedEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishEvent(ctx, claim, event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(j.WorkingDir, j.JobId.String()+".json")
	j, err = job.ReadContextFromDiskByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := recoverProtectedTerminal(ctx, j, "node", client); !errors.Is(err, serverclient.ErrProtectedConflict) {
		t.Fatalf("recovery before old lease expires = %v", err)
	}
	if delay := time.Until(expiry) + 50*time.Millisecond; delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := recoverProtectedTerminal(ctx, j, "node", client); err != nil {
		t.Fatalf("recover after lease expiry: %v", err)
	}
	stored, err := repo.GetJob(ctx, video.Id.String())
	if err != nil || stored.Events.GetStatus() != model.FailedNotificationStatus {
		t.Fatalf("recovered failure = %+v, %v", stored, err)
	}
	phaseFailures := 0
	for _, e := range stored.Events {
		if e.NotificationType == model.FFMPEGSNotification && e.Status == model.FailedNotificationStatus {
			phaseFailures++
		}
	}
	if phaseFailures != 1 {
		t.Fatalf("phase failures = %d, want 1", phaseFailures)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal workspace was not cleaned: %v", err)
	}
}
