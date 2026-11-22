package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"transcoder/model"
	"transcoder/server/repository"
)

func TestNotificationReplayAndAssignmentFencing(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repo := repository.NewInMemoryRepository()
	if err := repo.AddJob(ctx, &model.Job{Id: id, SourcePath: "movie.mkv", TargetPath: "movie.av1.mkv"}); err != nil {
		t.Fatal(err)
	}
	add := func(seq int, state model.NotificationStatus) {
		t.Helper()
		err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: seq, NotificationType: model.JobNotification, Status: state})
		if err != nil {
			t.Fatal(err)
		}
	}
	add(0, model.AssignedNotificationStatus)
	add(1, model.StartedNotificationStatus)
	scheduler := &RuntimeScheduler{repo: repo, config: &Config{SourcePath: t.TempDir()}}
	gen := 0
	event := model.TaskEventType{JobId: id, Event: model.Event{WorkerName: "worker"}, EventID: 2, AssignmentEventID: &gen, NotificationType: model.FFMPEGSNotification, Status: model.CompletedNotificationStatus}
	send := func(e model.TaskEventType) error {
		b, err := json.Marshal(e)
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
	current, err := repo.GetJob(ctx, id.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Events) != 3 {
		t.Fatalf("stored events = %d, want 3", len(current.Events))
	}
	conflicting := event
	conflicting.Message = "different"
	if err := send(conflicting); !errors.Is(err, ErrNotificationConflict) {
		t.Fatalf("conflict = %v", err)
	}
	add(3, model.QueuedNotificationStatus)
	add(4, model.AssignedNotificationStatus)
	add(5, model.StartedNotificationStatus)
	stale := event
	stale.EventID = 6 // exactly the next sequence, but belonging to the previous assignment
	if err := send(stale); !errors.Is(err, ErrAssignmentExpired) {
		t.Fatalf("stale = %v", err)
	}
}
