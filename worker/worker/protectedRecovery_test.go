package worker

import (
	"context"
	"os"
	"testing"
	"time"

	"transcoder/model"
)

type fakeProtectedRecoveryClient struct {
	leases []model.AssignmentClaim
	parks  []model.AssignmentClaim
	events []model.TaskEventType
}

func (f *fakeProtectedRecoveryClient) AcquireLease(_ context.Context, claim model.AssignmentClaim) (time.Time, error) {
	f.leases = append(f.leases, claim)
	return time.Now().Add(time.Minute), nil
}

func (f *fakeProtectedRecoveryClient) Park(_ context.Context, claim model.AssignmentClaim) error {
	f.parks = append(f.parks, claim)
	return nil
}

func (f *fakeProtectedRecoveryClient) PublishEvent(_ context.Context, _ model.AssignmentClaim, event model.TaskEventType) error {
	f.events = append(f.events, event)
	return nil
}

func TestRecoverParkedJobPreservesStopAndResumesSamePrefetch(t *testing.T) {
	for _, stopped := range []bool{true, false} {
		t.Run(map[bool]string{true: "stopped", false: "resumed"}[stopped], func(t *testing.T) {
			j := testParkableContext(t)
			oldInstance := j.Assignment.InstanceID
			j.Parked = true
			if err := j.PersistJobContext(); err != nil {
				t.Fatal(err)
			}
			client := &fakeProtectedRecoveryClient{}
			guard, err := recoverParkedJob(t.Context(), j, "node", client, stopped, time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if guard != nil {
				defer guard.Close()
			}
			if len(client.leases) != 1 || client.leases[0].InstanceID == oldInstance || j.Assignment.InstanceID != client.leases[0].InstanceID || j.NextPhase != model.FFMPEGSNotification {
				t.Fatalf("recovered lease = %+v, context = %+v", client.leases, j.Assignment)
			}
			if stopped && (!j.Parked || guard != nil || len(client.parks) != 1) {
				t.Fatalf("stopped recovery: parked=%t guard=%v parks=%d", j.Parked, guard, len(client.parks))
			}
			if !stopped && (j.Parked || guard == nil || len(client.parks) != 0) {
				t.Fatalf("resumed recovery: parked=%t guard=%v parks=%d", j.Parked, guard, len(client.parks))
			}
		})
	}
}

func TestRecoverParkedJobRejectsMissingSourceBeforeLease(t *testing.T) {
	j := testParkableContext(t)
	j.Parked = true
	if err := os.Remove(j.Source.FilePath); err != nil {
		t.Fatal(err)
	}
	client := &fakeProtectedRecoveryClient{}
	if _, err := recoverParkedJob(t.Context(), j, "node", client, false, time.Millisecond); err == nil || len(client.leases) != 0 {
		t.Fatalf("recovery with missing source = %v, leases=%d", err, len(client.leases))
	}
}
