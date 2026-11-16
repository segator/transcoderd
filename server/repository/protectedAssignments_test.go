package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/assignment"

	"github.com/google/uuid"
)

func TestProtectedRequestIsIdempotent(t *testing.T) {
	repo := NewInMemoryRepository()
	keys, err := assignment.NewKeyring("one", map[string][]byte{"one": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first := &model.Job{Id: uuid.New(), SourcePath: "movie.mkv"}
	if err := repo.AddJob(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, first.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	const callers = 20
	results := make([]*model.ProtectedAssignment, callers)
	errors := make([]error, callers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errors[i] = repo.RequestProtectedJob(ctx, "worker", requestID, keys)
		}()
	}
	wg.Wait()
	for i, got := range results {
		if errors[i] != nil || got == nil || got.Id != first.Id || got.Generation != 1 || got.Capability != results[0].Capability {
			t.Fatalf("RequestProtectedJob() caller %d = %+v, %v", i, got, errors[i])
		}
	}
	stored, err := repo.GetJob(ctx, first.Id.String())
	if err != nil || len(stored.Events) != 2 {
		t.Fatalf("stored events = %+v, %v; want queued and assigned", stored, err)
	}
	emptyRequest := uuid.NewString()
	if got, err := repo.RequestProtectedJob(ctx, "worker", emptyRequest, keys); err != nil || got != nil {
		t.Fatalf("empty request = %+v, %v", got, err)
	}
	second := &model.Job{Id: uuid.New(), SourcePath: "second.mkv"}
	if err := repo.AddJob(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, second.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.RequestProtectedJob(ctx, "worker", emptyRequest, keys); err != nil || got != nil {
		t.Fatalf("retried empty request = %+v, %v; want same empty result", got, err)
	}
	if got, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys); err != nil || got == nil || got.Id != second.Id {
		t.Fatalf("new request = %+v, %v; want second job", got, err)
	}
}

func testProtectedParking(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv", TargetPath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys)
	if err != nil || assigned == nil || assigned.Id != job.Id {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	job, err = repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := job.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assigned.Generation, Capability: assigned.Capability}
	acquireTestLease(t, repo, "worker", &claim)
	for i := 0; i < 2; i++ {
		if err := repo.ApplyProtectedEvent(ctx, "worker", claim, *started); err != nil {
			t.Fatalf("ApplyProtectedEvent() attempt %d = %v", i, err)
		}
	}
	progress := model.TaskProgressType{Event: model.Event{WorkerName: "worker", EventTime: time.Now().UTC()}, JobId: job.Id, ProgressID: uuid.NewString(), NotificationType: model.DownloadNotification, Status: model.ProgressingTaskProgressTypeStatus, Percent: 40}
	for i := 0; i < 2; i++ {
		if err := repo.ApplyProtectedProgress(ctx, "worker", claim, progress, 1); err != nil {
			t.Fatalf("ApplyProtectedProgress() attempt %d = %v", i, err)
		}
	}
	nextProgress := progress
	nextProgress.Percent = 80
	if err := repo.ApplyProtectedProgress(ctx, "worker", claim, nextProgress, 2); err != nil {
		t.Fatalf("ApplyProtectedProgress(newer) = %v", err)
	}
	if err := repo.ApplyProtectedProgress(ctx, "worker", claim, progress, 1); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("ApplyProtectedProgress(out of order) = %v", err)
	}
	nextProgress.Percent = 60
	if err := repo.ApplyProtectedProgress(ctx, "worker", claim, nextProgress, 2); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("ApplyProtectedProgress(conflicting retry) = %v", err)
	}
	conflicting := *started
	conflicting.Message = "different"
	if err := repo.ApplyProtectedEvent(ctx, "worker", claim, conflicting); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("ApplyProtectedEvent(conflicting) = %v, want event conflict", err)
	}
	if err := repo.ParkProtectedJob(ctx, "other", claim); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("ParkProtectedJob(other) = %v, want stale assignment", err)
	}
	if err := repo.ParkProtectedJob(ctx, "worker", claim); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("ParkProtectedJob(before prefetch) = %v, want conflict", err)
	}
	addTestCompletedPrefetch(t, repo, "worker", claim)
	for i := 0; i < 2; i++ {
		if err := repo.ParkProtectedJob(ctx, "worker", claim); err != nil {
			t.Fatalf("ParkProtectedJob() attempt %d = %v", i, err)
		}
	}
	oldClaim := claim
	acquireTestLease(t, repo, "worker", &claim)
	if err := repo.ParkProtectedJob(ctx, "worker", oldClaim); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("ParkProtectedJob(old process) = %v, want stale assignment", err)
	}
	if err := repo.ApplyProtectedEvent(ctx, "worker", oldClaim, *started); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("ApplyProtectedEvent(old process) = %v, want stale assignment", err)
	}
	if err := repo.ApplyProtectedProgress(ctx, "worker", oldClaim, progress, 3); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("ApplyProtectedProgress(old process) = %v, want stale assignment", err)
	}
	if err := repo.ParkProtectedJob(ctx, "worker", claim); err != nil {
		t.Fatalf("ParkProtectedJob(reclaimed) = %v", err)
	}
	if requeued, err := repo.RequeueTimedOutJob(ctx, job.Id.String(), started.EventID, time.Now().Add(time.Hour)); err != nil || requeued {
		t.Fatalf("RequeueTimedOutJob(parked) = %t, %v", requeued, err)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); err != nil {
		t.Fatalf("CancelJobAssignment() = %v", err)
	}
	if err := repo.ApplyProtectedEvent(ctx, "worker", claim, *started); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("ApplyProtectedEvent(canceled) = %v, want stale assignment", err)
	}
	if err := repo.ParkProtectedJob(ctx, "worker", claim); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("ParkProtectedJob(canceled) = %v, want stale assignment", err)
	}
	job, err = repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	newAssignment, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys)
	if err != nil || newAssignment == nil || newAssignment.Id != job.Id || newAssignment.Generation != assigned.Generation+1 || newAssignment.Capability == assigned.Capability {
		t.Fatalf("reassigned job = %+v, %v; want next generation", newAssignment, err)
	}
}

func acquireTestLease(t *testing.T, repo Repository, worker string, claim *model.AssignmentClaim) {
	t.Helper()
	claim.InstanceID = uuid.New()
	if _, err := repo.AcquireProtectedLease(context.Background(), worker, *claim, claim.InstanceID, time.Minute); err != nil {
		t.Fatal(err)
	}
}

func addTestCompletedPrefetch(t *testing.T, repo Repository, worker string, claim model.AssignmentClaim) {
	t.Helper()
	ctx := context.Background()
	current, err := repo.GetJob(ctx, claim.JobID.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []model.NotificationStatus{model.StartedNotificationStatus, model.CompletedNotificationStatus} {
		event := current.AddEvent(model.DownloadNotification, status)
		event.WorkerName = worker
		if err := repo.ApplyProtectedEvent(ctx, worker, claim, *event); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProtectedParking(t *testing.T) { testProtectedParking(t, NewInMemoryRepository()) }

func testProtectedParkTimeoutRace(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{8}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv", TargetPath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys)
	if err != nil || assigned == nil || assigned.Id != job.Id {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	job, err = repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := job.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	if err := repo.AddNewTaskEvent(ctx, started); err != nil {
		t.Fatal(err)
	}
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assigned.Generation, Capability: assigned.Capability}
	acquireTestLease(t, repo, "worker", &claim)
	addTestCompletedPrefetch(t, repo, "worker", claim)
	start := make(chan struct{})
	var parkErr, timeoutErr error
	var requeued bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-start; parkErr = repo.ParkProtectedJob(ctx, "worker", claim) }()
	go func() {
		defer wg.Done()
		<-start
		requeued, timeoutErr = repo.RequeueTimedOutJob(ctx, job.Id.String(), started.EventID, time.Now().Add(time.Hour))
	}()
	close(start)
	wg.Wait()
	if timeoutErr != nil || (parkErr != nil && !errors.Is(parkErr, ErrStaleAssignment)) || (parkErr == nil) == requeued {
		t.Fatalf("park/timeout race = %v / %t, %v; want exactly one winner", parkErr, requeued, timeoutErr)
	}
}

func TestProtectedParkTimeoutRace(t *testing.T) {
	testProtectedParkTimeoutRace(t, NewInMemoryRepository())
}

func testProtectedUploadRevocation(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{9}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv", TargetPath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys)
	if err != nil || assigned == nil || assigned.Id != job.Id {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	job, err = repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := job.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assigned.Generation, Capability: assigned.Capability}
	acquireTestLease(t, repo, "worker", &claim)
	if err := repo.ApplyProtectedEvent(ctx, "worker", claim, *started); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("encoded"))
	checksum := hex.EncodeToString(sum[:])
	staged := 0
	stage := func() error { staged++; return nil }
	for i := 0; i < 2; i++ {
		if err := repo.StageProtectedUpload(ctx, "worker", claim, "stage/file", checksum, 7, stage); err != nil {
			t.Fatalf("StageProtectedUpload() attempt %d = %v", i, err)
		}
	}
	if err := repo.StageProtectedUpload(ctx, "worker", claim, "stage/other", checksum, 7, stage); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("StageProtectedUpload(conflicting) = %v, want conflict", err)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); err != nil {
		t.Fatal(err)
	}
	if err := repo.StageProtectedUpload(ctx, "worker", claim, "stage/file", checksum, 7, stage); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("StageProtectedUpload(canceled) = %v, want stale assignment", err)
	}
	if staged != 2 {
		t.Errorf("staging callback ran %d times, want 2 authorized attempts", staged)
	}
	pending, err := repo.ListPendingProtectedPublications(ctx)
	if err != nil || len(pending) != 1 || pending[0].JobID != job.Id || pending[0].State != "abandoned" || pending[0].DeleteSource {
		t.Fatalf("abandoned upload = %+v, %v", pending, err)
	}
	err = repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		_, _, err := tx.GetJobForMutation(ctx, job.Id.String())
		return err
	})
	if !errors.Is(err, ErrJobPublishing) {
		t.Fatalf("GetJobForMutation(pending cleanup) = %v, want ErrJobPublishing", err)
	}
	if err := repo.ClearProtectedSourceDeletion(ctx, job.Id.String(), claim.Generation); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.ListPendingProtectedPublications(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after abandoned cleanup = %+v, %v", pending, err)
	}
}

func TestProtectedUploadRevocation(t *testing.T) {
	testProtectedUploadRevocation(t, NewInMemoryRepository())
}

func testProtectedPublication(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{5}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv", TargetPath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assigned, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys)
	if err != nil || assigned == nil || assigned.Id != job.Id {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assigned, err)
	}
	job, err = repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assigned.Generation, Capability: assigned.Capability}
	acquireTestLease(t, repo, "worker", &claim)
	started := job.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	if err := repo.ApplyProtectedEvent(ctx, "worker", claim, *started); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("encoded"))
	if err := repo.StageProtectedUpload(ctx, "worker", claim, "stage/file", hex.EncodeToString(sum[:]), 7, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	complete := job.AddEvent(model.JobNotification, model.CompletedNotificationStatus)
	complete.WorkerName = "worker"
	publication, err := repo.BeginProtectedPublication(ctx, "worker", claim, *complete, true)
	if err != nil || publication == nil || publication.State != "publishing" {
		t.Fatalf("BeginProtectedPublication() = %+v, %v", publication, err)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); !errors.Is(err, ErrJobPublishing) {
		t.Fatalf("CancelJobAssignment(publishing) = %v, want ErrJobPublishing", err)
	}
	if retry, err := repo.BeginProtectedPublication(ctx, "worker", claim, *complete, true); err != nil || retry.State != "publishing" {
		t.Fatalf("BeginProtectedPublication(retry) = %+v, %v", retry, err)
	}
	conflicting := *complete
	conflicting.Message = "different"
	if _, err := repo.BeginProtectedPublication(ctx, "worker", claim, conflicting, true); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("BeginProtectedPublication(conflict) = %v, want ErrEventConflict", err)
	}
	pending, err := repo.ListPendingProtectedPublications(ctx)
	if err != nil || len(pending) != 1 || pending[0].JobID != job.Id {
		t.Fatalf("ListPendingProtectedPublications() = %+v, %v", pending, err)
	}
	probe := &model.MediaProbe{DurationSeconds: 12}
	if err := repo.FinishProtectedPublication(ctx, *publication, "final.mkv", 7, probe); err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishProtectedPublication(ctx, *publication, "final.mkv", 7, probe); err != nil {
		t.Fatalf("FinishProtectedPublication(retry) = %v", err)
	}
	stored, err := repo.GetJob(ctx, job.Id.String())
	if err != nil || stored.Events.GetStatus() != model.CompletedNotificationStatus || stored.TargetPath != "final.mkv" {
		t.Fatalf("completed job = %+v, %v", stored, err)
	}
	if err := repo.ClearProtectedSourceDeletion(ctx, job.Id.String(), publication.Generation); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.ListPendingProtectedPublications(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after deletion acknowledgement = %+v, %v", pending, err)
	}
}

func TestProtectedPublication(t *testing.T) { testProtectedPublication(t, NewInMemoryRepository()) }
