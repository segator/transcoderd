package worker

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type fakeProtectedEventClient struct {
	events []model.TaskEventType
	errors []error
}

func (f *fakeProtectedEventClient) PublishEvent(_ context.Context, _ model.AssignmentClaim, event model.TaskEventType) error {
	f.events = append(f.events, event)
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}

func TestProtectedPublisherRetriesIdenticalEvent(t *testing.T) {
	j := job.NewContext(uuid.New(), 3, t.TempDir())
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	client := &fakeProtectedEventClient{errors: []error{errors.New("response lost"), nil}}
	publisher := &protectedEventPublisher{client: client, workerName: "node", retryDelay: time.Millisecond}
	if err := publisher.Publish(t.Context(), j, model.DownloadNotification, model.StartedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	if len(client.events) != 2 || !reflect.DeepEqual(client.events[0], client.events[1]) || client.events[0].EventID != 4 {
		t.Fatalf("retried events = %+v", client.events)
	}
	if j.PendingProtectedEvent() != nil || j.LastEvent == nil || j.LastEvent.EventId != 4 {
		t.Fatal("event acknowledgement was not saved")
	}
}

func TestProtectedPublisherReconcilesPendingEventAfterRestart(t *testing.T) {
	path := t.TempDir()
	j := job.NewContext(uuid.New(), 5, path)
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	client := &fakeProtectedEventClient{errors: []error{serverclient.ErrProtectedConflict}}
	publisher := &protectedEventPublisher{client: client, workerName: "node", retryDelay: time.Millisecond}
	if err := publisher.Publish(t.Context(), j, model.DownloadNotification, model.StartedNotificationStatus, ""); !errors.Is(err, serverclient.ErrProtectedConflict) {
		t.Fatalf("Publish() = %v, want conflict", err)
	}
	reloaded, err := job.ReadContextFromDiskByPath(filepath.Join(path, j.JobId.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	client.errors = nil
	if err := publisher.Publish(t.Context(), reloaded, model.DownloadNotification, model.StartedNotificationStatus, ""); err != nil {
		t.Fatal(err)
	}
	if len(client.events) != 2 || !reflect.DeepEqual(client.events[0], client.events[1]) || reloaded.LastEvent.EventId != 6 || reloaded.PendingProtectedEvent() != nil {
		t.Fatalf("reconciled events = %+v, context = %+v", client.events, reloaded.LastEvent)
	}
}

func TestCompletionReconcilesExactEventAfterLostLeaseResponse(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed", false: "rejected"}[accepted], func(t *testing.T) {
			j := job.NewContext(uuid.New(), 3, t.TempDir())
			j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
			guard, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) { return time.Now().Add(time.Minute), nil }, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer guard.Close()
			responses := []error{serverclient.ErrProtectedConflict, nil}
			if !accepted {
				responses[1] = serverclient.ErrProtectedConflict
			}
			client := &fakeProtectedEventClient{errors: responses}
			execution := &protectedExecution{job: j, guard: guard, events: &protectedEventPublisher{client: client, workerName: "node"}}
			e := &JobExecutor{serviceCtx: t.Context(), protectedJobs: map[uuid.UUID]*protectedExecution{j.JobId: execution}}
			atomic.StoreUint32(&e.activeJobs, 1)
			e.finishProtectedSuccess(j)
			if len(client.events) != 2 || !reflect.DeepEqual(client.events[0], client.events[1]) {
				t.Fatalf("completion replay = %+v", client.events)
			}
			if accepted && (e.ActiveJobs() != 0 || e.recoveryError != "") {
				t.Fatalf("acknowledged completion remains blocked: active=%d error=%q", e.ActiveJobs(), e.recoveryError)
			}
			if !accepted && (e.ActiveJobs() != 1 || e.recoveryError == "") {
				t.Fatalf("rejected completion was treated as successful: active=%d error=%q", e.ActiveJobs(), e.recoveryError)
			}
		})
	}
}
