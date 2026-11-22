//go:build integration

package repository

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"transcoder/model"
)

func TestLegacyNotificationDigestAndJobLockPostgres(t *testing.T) {
	dsn := os.Getenv("DASHBOARD_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("DASHBOARD_TEST_POSTGRES is not set")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "events_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &SQLRepository{db: &SQLDatabase{db: db}}
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := repo.AddJob(ctx, &model.Job{Id: id, SourcePath: "movie.mkv", TargetPath: "out.mkv"}); err != nil {
		t.Fatal(err)
	}
	event := &model.TaskEventType{JobId: id, EventID: 0, Event: model.Event{WorkerName: "worker", EventTime: time.Now().UTC()}, NotificationType: model.JobNotification, Status: model.AssignedNotificationStatus}
	if err := repo.AddNewTaskEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetJob(ctx, id.String())
	if err != nil {
		t.Fatal(err)
	}
	digest, err := model.NotificationDigest(event)
	if err != nil {
		t.Fatal(err)
	}
	if got.Events[0].DeliveryDigest != digest {
		t.Fatalf("digest = %q, want %q", got.Events[0].DeliveryDigest, digest)
	}
	if _, err := repo.GetTimeoutJobs(ctx, time.Second); err != nil {
		t.Fatalf("timeout query after migration: %v", err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
			if err := tx.LockJob(ctx, id.String()); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: id, EventID: 1, Event: model.Event{WorkerName: "worker"}, NotificationType: model.JobNotification, Status: model.StartedNotificationStatus})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("event bypassed job lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}
