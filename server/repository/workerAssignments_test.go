package repository

import (
	"bytes"
	"context"
	"testing"

	"transcoder/model"
	"transcoder/server/assignment"

	"github.com/google/uuid"
)

func testWorkerAssignmentInventory(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	legacy := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, legacy.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	legacyAssigned := legacy.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
	legacyAssigned.WorkerName = "inventory-worker"
	if err := repo.AddNewTaskEvent(ctx, legacyAssigned); err != nil {
		t.Fatal(err)
	}
	protected := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, protected); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, protected.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{2}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "inventory-worker", uuid.NewString(), keys)
	if err != nil || assigned == nil || assigned.Id != protected.Id {
		t.Fatalf("protected assignment = %+v, %v", assigned, err)
	}
	items, err := repo.ListWorkerAssignments(ctx, "inventory-worker")
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[uuid.UUID]model.WorkerAssignmentSnapshot)
	for _, item := range items {
		found[item.JobID] = item
	}
	if found[legacy.Id].State != "legacy" || found[protected.Id].State != "assigned" || found[protected.Id].Generation != assigned.Generation {
		t.Fatalf("worker assignment inventory = %+v", items)
	}
	if other, err := repo.ListWorkerAssignments(ctx, "some-other-worker"); err != nil || len(other) != 0 {
		t.Fatalf("other worker inventory = %+v, %v", other, err)
	}
}

func TestWorkerAssignmentInventory(t *testing.T) {
	testWorkerAssignmentInventory(t, NewInMemoryRepository())
}
