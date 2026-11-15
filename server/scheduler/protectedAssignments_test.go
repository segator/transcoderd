package scheduler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/assignment"
	"transcoder/server/repository"

	"github.com/google/uuid"
)

func TestProtectedAssignmentRejectsLegacyTraffic(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryRepository()
	scheduler, err := NewScheduler(&Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: "film.mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	protected, err := repo.RequestProtectedJob(ctx, "node", uuid.NewString(), keys)
	if err != nil || protected == nil {
		t.Fatalf("RequestProtectedJob() = %+v, %v", protected, err)
	}
	legacyEvent := model.TaskEventType{JobId: job.Id, EventID: protected.EventID + 1, Event: model.Event{WorkerName: "node"}, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
	data, err := json.Marshal(legacyEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.HandleWorkerEvent(ctx, &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: data}); !errors.Is(err, repository.ErrStaleAssignment) {
		t.Fatalf("HandleWorkerEvent() = %v, want stale assignment", err)
	}
	if _, err := scheduler.GetUploadJobWriter(ctx, job.Id.String(), "node"); !errors.Is(err, ErrorStreamNotAllowed) {
		t.Fatalf("GetUploadJobWriter() = %v, want ErrorStreamNotAllowed", err)
	}
	if _, err := scheduler.GetDownloadJobWriter(ctx, job.Id.String(), "node"); !errors.Is(err, ErrorStreamNotAllowed) {
		t.Fatalf("GetDownloadJobWriter() = %v, want ErrorStreamNotAllowed", err)
	}
	stored, err := repo.GetJob(ctx, job.Id.String())
	if err != nil || len(stored.Events) != 2 {
		t.Fatalf("events after rejected traffic = %+v, %v", stored, err)
	}
}

func TestProtectedUploadCannotCommitAfterCancellation(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryRepository()
	scheduler, err := NewScheduler(&Config{SourcePath: t.TempDir()}, repo)
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: "film.mkv", TargetPath: "film-av1.mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "node", uuid.NewString(), keys)
	if err != nil || assigned == nil {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assigned.Generation, Capability: assigned.Capability}
	claim.InstanceID = uuid.New()
	if _, err := repo.AcquireProtectedLease(ctx, "node", claim, claim.InstanceID, time.Minute); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := current.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "node"
	if err := repo.ApplyProtectedEvent(ctx, "node", claim, *started); err != nil {
		t.Fatal(err)
	}
	stream, err := scheduler.GetProtectedUpload(ctx, "node", claim)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Clean()
	data := []byte("encoded")
	if _, err := stream.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(false); err != nil {
		t.Fatal(err)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := scheduler.StageProtectedUpload(ctx, "node", claim, stream, hex.EncodeToString(sum[:]), int64(len(data))); !errors.Is(err, repository.ErrStaleAssignment) {
		t.Fatalf("StageProtectedUpload() = %v, want stale assignment", err)
	}
	if _, err := os.Stat(filepath.Join(scheduler.config.SourcePath, stream.stagingName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging file after cancellation: %v, want not exist", err)
	}
}

func TestRecoverAbandonedUploadPreservesSource(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryRepository()
	root := t.TempDir()
	scheduler, err := NewScheduler(&Config{SourcePath: root, DeleteSourceOnComplete: true}, repo)
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: "source.mkv", TargetPath: "target.mkv"}
	if err := os.WriteFile(filepath.Join(root, job.SourcePath), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "node", uuid.NewString(), keys)
	if err != nil || assigned == nil {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assigned.Generation, Capability: assigned.Capability}
	claim.InstanceID = uuid.New()
	if _, err := repo.AcquireProtectedLease(ctx, "node", claim, claim.InstanceID, time.Minute); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := current.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "node"
	if err := repo.ApplyProtectedEvent(ctx, "node", claim, *started); err != nil {
		t.Fatal(err)
	}
	stream, err := scheduler.GetProtectedUpload(ctx, "node", claim)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Clean()
	data := []byte("encoded")
	if _, err := stream.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(false); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := scheduler.StageProtectedUpload(ctx, "node", claim, stream, hex.EncodeToString(sum[:]), int64(len(data))); err != nil {
		t.Fatal(err)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.recoverProtectedPublications(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, stream.stagingName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned staged output = %v, want absent", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, job.SourcePath)); err != nil || string(data) != "source" {
		t.Fatalf("source after cleanup = %q, %v", data, err)
	}
}
