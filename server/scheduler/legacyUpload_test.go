package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"transcoder/model"
	"transcoder/server/repository"
)

func TestUploadCannotCommitAfterReassignment(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "movies"), 0700); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	repo := repository.NewInMemoryRepository()
	if err := repo.AddJob(ctx, &model.Job{Id: id, SourcePath: "movies/source.mkv", TargetPath: "encoded.mkv"}); err != nil {
		t.Fatal(err)
	}
	add := func(sequence int, worker string, status model.NotificationStatus) {
		t.Helper()
		err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, Event: model.Event{WorkerName: worker}, EventID: sequence, NotificationType: model.JobNotification, Status: status})
		if err != nil {
			t.Fatal(err)
		}
	}
	add(1, "same-worker", model.AssignedNotificationStatus)
	add(2, "same-worker", model.StartedNotificationStatus)
	scheduler := &RuntimeScheduler{repo: repo, config: &Config{SourcePath: root}}
	assignment := 1
	stream, err := scheduler.GetUploadJobWriter(ctx, id.String(), "same-worker", &assignment)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Clean()
	if _, err := stream.Write([]byte("old result")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(false); err != nil {
		t.Fatal(err)
	}
	add(3, "", model.QueuedNotificationStatus)
	add(4, "same-worker", model.AssignedNotificationStatus)
	add(5, "same-worker", model.StartedNotificationStatus)
	if err := stream.Commit(); !errors.Is(err, ErrAssignmentExpired) {
		t.Fatalf("Commit() = %v, want assignment expired", err)
	}
	if _, err := scheduler.GetUploadJobWriter(ctx, id.String(), "same-worker", &assignment); !errors.Is(err, ErrAssignmentExpired) {
		t.Fatalf("stale new upload = %v, want assignment expired", err)
	}
}

func TestReassignmentRemovesPreviousStagedUpload(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	id := uuid.New()
	repo := repository.NewInMemoryRepository()
	video := &model.Job{Id: id, SourcePath: "source.mkv", TargetPath: "encoded.mkv"}
	if err := repo.AddJob(ctx, video); err != nil {
		t.Fatal(err)
	}
	for seq, status := range []model.NotificationStatus{model.AssignedNotificationStatus, model.StartedNotificationStatus} {
		if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: seq, NotificationType: model.JobNotification, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	assignmentID := 0
	stage, err := uploadAssignmentStagingPath(root, video.SourcePath, id.String(), &assignmentID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stage), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("old encoded output"), 0600); err != nil {
		t.Fatal(err)
	}
	scheduler := &RuntimeScheduler{repo: repo, config: &Config{SourcePath: root}}
	_, err = scheduler.scheduleJobRequest(ctx, &model.JobRequest{SourcePath: video.SourcePath, TargetPath: video.TargetPath, ForceAssigned: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old staged output remains: %v", err)
	}
}

func TestAssignedCompletionDoesNotUseLegacyStaging(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	id := uuid.New()
	repo := repository.NewInMemoryRepository()
	video := &model.Job{Id: id, SourcePath: "source.mkv", TargetPath: "source.mkv"}
	if err := repo.AddJob(ctx, video); err != nil {
		t.Fatal(err)
	}
	for seq, status := range []model.NotificationStatus{model.AssignedNotificationStatus, model.StartedNotificationStatus} {
		if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: seq, NotificationType: model.JobNotification, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, video.SourcePath), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy, err := uploadStagingPath(root, video.SourcePath, id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("old assignment"), 0600); err != nil {
		t.Fatal(err)
	}
	generation := 0
	event := model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: 2, AssignmentEventID: &generation, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &RuntimeScheduler{repo: repo, config: &Config{SourcePath: root}}
	if err := scheduler.HandleWorkerEvent(ctx, &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: body}); err == nil {
		t.Fatal("completion succeeded without this assignment's upload")
	}
	stored, err := repo.GetJob(ctx, id.String())
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Events.GetLatest().EventID; got != 1 {
		t.Fatalf("last event = %d, want 1", got)
	}
}

func TestAssignedCompletionPublishesAndReplays(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX ffprobe script")
	}
	ctx := context.Background()
	root, bin := t.TempDir(), t.TempDir()
	probe := filepath.Join(bin, "ffprobe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"av1\"}]}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	id := uuid.New()
	video := &model.Job{Id: id, SourcePath: "source.mkv", TargetPath: "source.mkv"}
	repo := repository.NewInMemoryRepository()
	if err := repo.AddJob(ctx, video); err != nil {
		t.Fatal(err)
	}
	for seq, status := range []model.NotificationStatus{model.AssignedNotificationStatus, model.StartedNotificationStatus} {
		if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: seq, NotificationType: model.JobNotification, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, video.SourcePath), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	assignment := 0
	stage, err := uploadAssignmentStagingPath(root, video.SourcePath, id.String(), &assignment)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stage), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("encoded"), 0600); err != nil {
		t.Fatal(err)
	}
	event := model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: 2, AssignmentEventID: &assignment, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	envelope := &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: body}
	scheduler := &RuntimeScheduler{repo: repo, config: &Config{SourcePath: root}}
	if err := scheduler.HandleWorkerEvent(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.HandleWorkerEvent(ctx, envelope); err != nil {
		t.Fatalf("replay: %v", err)
	}
	stored, err := repo.GetJob(ctx, id.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Events) != 3 || stored.Status != string(model.CompletedNotificationStatus) {
		t.Fatalf("stored = %+v", stored)
	}
	data, err := os.ReadFile(filepath.Join(root, stored.TargetPath))
	if err != nil || string(data) != "encoded" {
		t.Fatalf("output=%q err=%v", data, err)
	}
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging remains: %v", err)
	}
}
