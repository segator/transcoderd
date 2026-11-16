package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"transcoder/model"
	"transcoder/worker/job"

	"github.com/google/uuid"
)

type fakeProtectedRequestClient struct {
	assignment *model.ProtectedAssignment
	errors     []error
	requests   []uuid.UUID
}

func (f *fakeProtectedRequestClient) RequestJob(_ context.Context, id uuid.UUID) (*model.ProtectedAssignment, error) {
	f.requests = append(f.requests, id)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return nil, err
		}
	}
	return f.assignment, nil
}

func TestProtectedAdmissionReusesRequestAfterLostResponse(t *testing.T) {
	workspace, err := job.AcquireWorkspaceLock(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	assigned := &model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: uuid.New(), EventID: 2, Name: "movie.mkv"}, Generation: 1, KeyID: "v1", Capability: "secret"}
	client := &fakeProtectedRequestClient{assignment: assigned, errors: []error{errors.New("response lost"), nil}}
	prepare := func(*job.Context) error { return nil }
	if _, err := requestProtectedJob(t.Context(), workspace, client, prepare); err == nil {
		t.Fatal("expected uncertain request to remain pending")
	}
	got, err := requestProtectedJob(t.Context(), workspace, client, prepare)
	if err != nil || got == nil || got.JobId != assigned.Id || got.Assignment == nil || got.Assignment.Capability != "secret" || len(client.requests) != 2 || client.requests[0] != client.requests[1] || got.RequestID != client.requests[0] {
		t.Fatalf("retried assignment = %+v, %v; requests = %v", got, err, client.requests)
	}
	stored, err := job.ReadContextFromDiskByPath(filepath.Join(got.WorkingDir, got.JobId.String()+".json"))
	if err != nil || stored.RequestID != got.RequestID || stored.Assignment == nil || stored.Assignment.Generation != 1 {
		t.Fatalf("stored assignment = %+v, %v", stored, err)
	}
}

func TestProtectedAdmissionPreservesExistingCheckpoint(t *testing.T) {
	root := t.TempDir()
	workspace, err := job.AcquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	requestID, err := workspace.PendingRequest()
	if err != nil {
		t.Fatal(err)
	}
	assigned := &model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: uuid.New(), EventID: 2}, Generation: 1, KeyID: "v1", Capability: "secret"}
	existing := job.NewContext(assigned.Id, assigned.EventID, filepath.Join(root, assigned.Id.String()))
	existing.Assignment = &model.AssignmentClaim{JobID: assigned.Id, Generation: 1, Capability: "secret"}
	existing.RequestID = requestID
	existing.NextPhase = model.FFMPEGSNotification
	if err := existing.Init(); err != nil {
		t.Fatal(err)
	}
	if err := existing.PersistJobContext(); err != nil {
		t.Fatal(err)
	}
	client := &fakeProtectedRequestClient{assignment: assigned}
	got, err := requestProtectedJob(t.Context(), workspace, client, func(*job.Context) error {
		t.Error("existing checkpoint should not be prepared or replaced")
		return nil
	})
	if err != nil || got == nil || got.NextPhase != model.FFMPEGSNotification || len(client.requests) != 1 || client.requests[0] != requestID {
		t.Fatalf("reconciled checkpoint = %+v, %v; requests = %v", got, err, client.requests)
	}
}

func TestStartupLeavesPendingRequestCheckpointForJournalReconciliation(t *testing.T) {
	root := t.TempDir()
	workspace, err := job.AcquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Close() }()
	requestID, err := workspace.PendingRequest()
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	checkpoint := job.NewContext(id, 1, filepath.Join(root, id.String()))
	checkpoint.RequestID = requestID
	checkpoint.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret"}
	if err := checkpoint.Init(); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.PersistJobContext(); err != nil {
		t.Fatal(err)
	}
	e := &JobExecutor{tempPath: root, protectedWorkspace: workspace, protectedRecoveryPending: true, reservedJobs: 1}
	e.resumeJobsContext(t.Context())
	if e.recoveryError != "" || e.ActiveJobs() != 0 || e.reservedJobs != 1 {
		t.Fatalf("pending checkpoint was treated as a second job: error=%q active=%d reserved=%d", e.recoveryError, e.ActiveJobs(), e.reservedJobs)
	}
	client := &fakeProtectedRequestClient{assignment: &model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: id, EventID: 1}, Generation: 1, KeyID: "v1", Capability: "secret"}}
	recovered, err := requestProtectedJob(t.Context(), workspace, client, func(*job.Context) error {
		t.Fatal("recovery must retain the existing assignment checkpoint")
		return nil
	})
	if err != nil || recovered == nil || recovered.RequestID != requestID || recovered.JobId != id {
		t.Fatalf("recovered assignment = %+v, %v", recovered, err)
	}
}
