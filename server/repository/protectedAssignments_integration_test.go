//go:build integration

package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"transcoder/model"
	"transcoder/server/assignment"

	"github.com/google/uuid"
)

func TestProtectedAssignmentsPostgres(t *testing.T) {
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
	schema := "protected_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	repo := &SQLRepository{db: &SQLDatabase{db: db}}
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: "film.mkv", TargetPath: "encoded.mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	const callers = 12
	results := make([]*model.ProtectedAssignment, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = repo.RequestProtectedJob(ctx, "worker", requestID, keys)
		}()
	}
	wg.Wait()
	for i, got := range results {
		if errs[i] != nil || got == nil || got.Id != job.Id || got.Generation != 1 || got.Capability != results[0].Capability {
			t.Fatalf("RequestProtectedJob() caller %d = %+v, %v", i, got, errs[i])
		}
	}
	loaded, err := repo.GetJob(ctx, job.Id.String())
	if err != nil || len(loaded.Events) != 2 {
		t.Fatalf("stored assignment = %+v, %v; want exactly two events", loaded, err)
	}
	emptyID := uuid.NewString()
	if got, err := repo.RequestProtectedJob(ctx, "worker", emptyID, keys); err != nil || got != nil {
		t.Fatalf("empty request = %+v, %v", got, err)
	}
	second := &model.Job{Id: uuid.New(), SourcePath: "second.mkv", TargetPath: "second-out.mkv"}
	if err := repo.AddJob(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, second.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.RequestProtectedJob(ctx, "worker", emptyID, keys); err != nil || got != nil {
		t.Fatalf("retried empty request = %+v, %v", got, err)
	}
	if got, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys); err != nil || got == nil || got.Id != second.Id {
		t.Fatalf("new request = %+v, %v; want second job", got, err)
	}

	third := &model.Job{Id: uuid.New(), SourcePath: "third.mkv", TargetPath: "third-out.mkv"}
	if err := repo.AddJob(ctx, third); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, third.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var protected *model.ProtectedAssignment
	var legacy *model.Job
	var protectedErr, legacyErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		protected, protectedErr = repo.RequestProtectedJob(ctx, "protected", uuid.NewString(), keys)
	}()
	go func() {
		defer wg.Done()
		<-start
		legacyErr = repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
			var err error
			legacy, err = tx.RetrieveQueuedJob(ctx)
			if err != nil {
				return err
			}
			event := legacy.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
			event.WorkerName = "legacy"
			return tx.AddNewTaskEvent(ctx, event)
		})
	}()
	close(start)
	wg.Wait()
	if protectedErr != nil || (legacyErr != nil && !errors.Is(legacyErr, ErrElementNotFound)) || (protected == nil) == (legacy == nil) {
		t.Fatalf("concurrent protected/legacy assignment = %+v %v / %+v %v; want exactly one winner", protected, protectedErr, legacy, legacyErr)
	}
	testProtectedParking(t, repo)
	testProtectedUploadRevocation(t, repo)
	testProtectedPublication(t, repo)
	testProtectedParkTimeoutRace(t, repo)
}
