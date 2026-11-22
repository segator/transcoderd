//go:build integration

package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"transcoder/model"
	"transcoder/server/repository"
)

func TestLegacyDeliveryPostgres(t *testing.T) {
	rawPort := os.Getenv("EVENT_TEST_POSTGRES_PORT")
	if rawPort == "" {
		t.Skip("EVENT_TEST_POSTGRES_PORT is not set")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewSQLRepository(&repository.SQLServerConfig{Host: "127.0.0.1", Port: port, User: "postgres", Password: "codex-test", Scheme: "postgres", Driver: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := repo.AddJob(ctx, &model.Job{Id: id, SourcePath: id.String() + ".mkv", TargetPath: id.String() + ".out.mkv"}); err != nil {
		t.Fatal(err)
	}
	add := func(sequence int, worker string, status model.NotificationStatus) {
		t.Helper()
		if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, EventID: sequence, Event: model.Event{WorkerName: worker}, NotificationType: model.JobNotification, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	add(0, "worker", model.AssignedNotificationStatus)
	add(1, "worker", model.StartedNotificationStatus)
	scheduler := &RuntimeScheduler{repo: repo, config: &Config{SourcePath: t.TempDir()}}
	gen := 0
	event := model.TaskEventType{JobId: id, EventID: 2, Event: model.Event{WorkerName: "worker", EventTime: time.Now().UTC()}, AssignmentEventID: &gen, NotificationType: model.FFMPEGSNotification, Status: model.CompletedNotificationStatus}
	send := func(event model.TaskEventType) error {
		b, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return scheduler.HandleWorkerEvent(ctx, &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: b})
	}
	if err := send(event); err != nil {
		t.Fatal(err)
	}
	if err := send(event); err != nil {
		t.Fatalf("replay: %v", err)
	}
	changed := event
	changed.EventTime = changed.EventTime.Add(time.Second)
	if err := send(changed); !errors.Is(err, ErrNotificationConflict) {
		t.Fatalf("changed replay = %v", err)
	}
	add(3, "", model.QueuedNotificationStatus)
	add(4, "worker", model.AssignedNotificationStatus)
	add(5, "worker", model.StartedNotificationStatus)
	event.EventID = 6
	if err := send(event); !errors.Is(err, ErrAssignmentExpired) {
		t.Fatalf("stale = %v", err)
	}

	root, bin := t.TempDir(), t.TempDir()
	probe := filepath.Join(bin, "ffprobe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"av1\"}]}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	scheduler.config.SourcePath = root
	newID := uuid.New()
	source := newID.String() + ".mkv"
	if err := os.WriteFile(filepath.Join(root, source), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddJob(ctx, &model.Job{Id: newID, SourcePath: source, TargetPath: source}); err != nil {
		t.Fatal(err)
	}
	for seq, status := range []model.NotificationStatus{model.AssignedNotificationStatus, model.StartedNotificationStatus} {
		if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: newID, EventID: seq, Event: model.Event{WorkerName: "worker"}, NotificationType: model.JobNotification, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	generation := 0
	stage, err := uploadAssignmentStagingPath(root, source, newID.String(), &generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stage), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("encoded"), 0600); err != nil {
		t.Fatal(err)
	}
	complete := model.TaskEventType{JobId: newID, EventID: 2, Event: model.Event{WorkerName: "worker", EventTime: time.Now().UTC()}, AssignmentEventID: &generation, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
	if err := send(complete); err != nil {
		t.Fatal(err)
	}
	if err := send(complete); err != nil {
		t.Fatalf("replay completed: %v", err)
	}
	stored, err := repo.GetJob(ctx, newID.String())
	if err != nil {
		t.Fatal(err)
	}
	if stored.TargetSize != int64(len("encoded")) || stored.Status != string(model.CompletedNotificationStatus) {
		t.Fatalf("completed job = %+v", stored)
	}
}
