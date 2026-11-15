package repository

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/assignment"

	"github.com/google/uuid"
)

func testProtectedLease(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	keys, err := assignment.NewKeyring("key", map[string][]byte{"key": bytes.Repeat([]byte{4}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddNewTaskEvent(ctx, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	assignment, err := repo.RequestProtectedJob(ctx, "worker", uuid.NewString(), keys)
	if err != nil || assignment == nil || assignment.Id != job.Id {
		t.Fatalf("RequestProtectedJob() = %+v, %v", assignment, err)
	}
	claim := model.AssignmentClaim{JobID: job.Id, Generation: assignment.Generation, Capability: assignment.Capability}
	instances := []uuid.UUID{uuid.New(), uuid.New()}
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.AcquireProtectedLease(ctx, "worker", claim, instances[i], time.Minute)
		}()
	}
	close(start)
	wg.Wait()
	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner != -1 {
				t.Fatal("two processes acquired the same assignment lease")
			}
			winner = i
		} else if !errors.Is(err, ErrLeaseHeld) {
			t.Fatalf("AcquireProtectedLease() = %v", err)
		}
	}
	if winner == -1 {
		t.Fatal("no process acquired the lease")
	}
	if expiry, err := repo.AcquireProtectedLease(ctx, "worker", claim, instances[winner], time.Minute); err != nil || expiry.IsZero() {
		t.Fatalf("renew lease = %v, %v", expiry, err)
	}
	if _, err := repo.AcquireProtectedLease(ctx, "other", claim, instances[winner], time.Minute); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("lease by wrong worker = %v", err)
	}
	// Advance the persisted deadline without sleeping, then verify that the
	// old process cannot submit events after a new process takes ownership.
	switch r := repo.(type) {
	case *SQLRepository:
		conn, err := r.getConnection()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, `UPDATE protected_assignments SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE job_id=$1`, job.Id.String()); err != nil {
			t.Fatal(err)
		}
	case *InMemoryRepository:
		r.mu.Lock()
		state := r.protectedJobs[job.Id.String()]
		state.leaseExpiry = time.Now().Add(-time.Second)
		r.protectedJobs[job.Id.String()] = state
		r.mu.Unlock()
	default:
		t.Fatal("test does not know how to expire this repository's lease")
	}
	newInstance := instances[1-winner]
	if _, err := repo.AcquireProtectedLease(ctx, "worker", claim, newInstance, time.Minute); err != nil {
		t.Fatalf("acquire expired lease = %v", err)
	}
	stored, err := repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	started := stored.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	started.WorkerName = "worker"
	claim.InstanceID = instances[winner]
	if err := repo.ApplyProtectedEvent(ctx, "worker", claim, *started); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("event from expired lease = %v", err)
	}
	claim.InstanceID = newInstance
	if err := repo.ApplyProtectedEvent(ctx, "worker", claim, *started); err != nil {
		t.Fatalf("event from current lease = %v", err)
	}
	if requeued, err := repo.RequeueTimedOutJob(ctx, job.Id.String(), started.EventID, time.Now().Add(time.Hour)); err != nil || requeued {
		t.Fatalf("timeout with current lease = %t, %v", requeued, err)
	}
	if err := repo.CancelJobAssignment(ctx, job.Id.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AcquireProtectedLease(ctx, "worker", claim, instances[winner], time.Minute); !errors.Is(err, ErrStaleAssignment) {
		t.Fatalf("lease after cancellation = %v", err)
	}
}

func TestProtectedLease(t *testing.T) { testProtectedLease(t, NewInMemoryRepository()) }
