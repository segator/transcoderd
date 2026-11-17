package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"

	"github.com/google/uuid"
)

type fakeProtectedRecoveryClient struct {
	leases      []model.AssignmentClaim
	leaseErrors []error
	parks       []model.AssignmentClaim
	events      []model.TaskEventType
}

type terminalRecoveryClient struct {
	claims   []model.AssignmentClaim
	events   []model.TaskEventType
	errors   []error
	leaseErr error
	leases   []model.AssignmentClaim
}

func (f *terminalRecoveryClient) PublishEvent(_ context.Context, claim model.AssignmentClaim, event model.TaskEventType) error {
	f.claims = append(f.claims, claim)
	f.events = append(f.events, event)
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}

func (f *terminalRecoveryClient) AcquireLease(_ context.Context, claim model.AssignmentClaim) (time.Time, error) {
	f.leases = append(f.leases, claim)
	return time.Now().Add(time.Minute), f.leaseErr
}

func (f *terminalRecoveryClient) Park(context.Context, model.AssignmentClaim) error { return nil }

func TestRecoverProtectedTerminalCompletion(t *testing.T) {
	for _, tt := range []struct {
		name     string
		errors   []error
		leaseErr error
		wantErr  bool
		leases   int
	}{
		{name: "server already committed", errors: []error{nil}},
		{name: "retry after new lease", errors: []error{serverclient.ErrProtectedConflict, nil}, leases: 1},
		{name: "lease unavailable", errors: []error{serverclient.ErrProtectedConflict}, leaseErr: serverclient.ErrProtectedConflict, wantErr: true, leases: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			path := filepath.Join(t.TempDir(), id.String())
			j := job.NewContext(id, 5, path)
			j.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
			j.RequestID = uuid.New()
			if err := j.Init(); err != nil {
				t.Fatal(err)
			}
			event, err := j.NewProtectedEvent("node", model.JobNotification, model.CompletedNotificationStatus, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := j.StageProtectedEvent(event); err != nil {
				t.Fatal(err)
			}
			client := &terminalRecoveryClient{errors: tt.errors, leaseErr: tt.leaseErr}
			err = recoverProtectedTerminal(t.Context(), j, "node", client)
			if (err != nil) != tt.wantErr || len(client.leases) != tt.leases {
				t.Fatalf("recover completion = %v, leases=%d", err, len(client.leases))
			}
			_, statErr := os.Stat(path)
			if tt.wantErr && statErr != nil || !tt.wantErr && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("workspace after recovery = %v", statErr)
			}
			if len(client.events) == 2 && (!reflect.DeepEqual(client.events[0], client.events[1]) || client.claims[0].InstanceID == client.claims[1].InstanceID) {
				t.Fatal("completion event or process identity was not reconciled correctly")
			}
		})
	}
}

func TestRecoverProtectedTerminalFailure(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(fmt.Sprintf("server_committed=%t", accepted), func(t *testing.T) {
			id := uuid.New()
			path := filepath.Join(t.TempDir(), id.String())
			j := job.NewContext(id, 5, path)
			j.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
			j.RequestID = uuid.New()
			if err := j.Init(); err != nil {
				t.Fatal(err)
			}
			event, err := j.NewProtectedEvent("node", model.JobNotification, model.FailedNotificationStatus, "FFMPEG:encoder failed")
			if err != nil {
				t.Fatal(err)
			}
			if err := j.StageProtectedEvent(event); err != nil {
				t.Fatal(err)
			}
			client := &terminalRecoveryClient{}
			if !accepted {
				client.errors = []error{serverclient.ErrProtectedConflict, nil}
			}
			if err := recoverProtectedTerminal(t.Context(), j, "node", client); err != nil {
				t.Fatal(err)
			}
			if len(client.leases) != boolInt(!accepted) || len(client.events) != 1+boolInt(!accepted) {
				t.Fatalf("failure recovery leases=%d events=%d", len(client.leases), len(client.events))
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("terminal workspace was not cleaned: %v", err)
			}
		})
	}
}

func TestRecoverDefinitivePhaseFailure(t *testing.T) {
	for _, checkpoint := range []string{"before phase event", "phase event pending", "phase event acknowledged"} {
		t.Run(checkpoint, func(t *testing.T) {
			id := uuid.New()
			path := filepath.Join(t.TempDir(), id.String())
			j := job.NewContext(id, 5, path)
			j.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
			j.RequestID = uuid.New()
			j.NextPhase = model.FFMPEGSNotification
			j.LastEvent = &job.ContextEvent{EventId: 6, NotificationType: model.FFMPEGSNotification, Status: model.StartedNotificationStatus}
			if err := j.Init(); err != nil {
				t.Fatal(err)
			}
			if err := j.RecordPhaseFailure(model.FFMPEGSNotification, "encoder failed"); err != nil {
				t.Fatal(err)
			}
			if checkpoint != "before phase event" {
				event, err := j.NewProtectedEvent("node", model.FFMPEGSNotification, model.FailedNotificationStatus, "encoder failed")
				if err != nil {
					t.Fatal(err)
				}
				if err := j.StageProtectedEvent(event); err != nil {
					t.Fatal(err)
				}
				if checkpoint == "phase event acknowledged" {
					if err := j.AcknowledgeProtectedEvent(event.EventID); err != nil {
						t.Fatal(err)
					}
				}
			}
			j, err := job.ReadContextFromDiskByPath(filepath.Join(path, id.String()+".json"))
			if err != nil || !recoverableTerminalAssignment(j) {
				t.Fatalf("recoverable checkpoint = %+v, %v", j, err)
			}
			client := &fakeProtectedRecoveryClient{}
			if err := recoverProtectedTerminal(t.Context(), j, "node", client); err != nil {
				t.Fatal(err)
			}
			if len(client.leases) != 1 || len(client.events) != 1+boolInt(checkpoint != "phase event acknowledged") {
				t.Fatalf("recovery leases=%d events=%+v", len(client.leases), client.events)
			}
			last := client.events[len(client.events)-1]
			if last.NotificationType != model.JobNotification || last.Status != model.FailedNotificationStatus || last.Message != "FFMPEG:encoder failed" {
				t.Fatalf("terminal failure = %+v", last)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("terminal workspace was not cleaned: %v", err)
			}
		})
	}
}

func TestRecoverDefinitivePhaseFailureRequiresLease(t *testing.T) {
	id := uuid.New()
	j := job.NewContext(id, 5, t.TempDir())
	j.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	j.RequestID = uuid.New()
	j.NextPhase = model.DownloadNotification
	j.LastEvent = &job.ContextEvent{EventId: 6, NotificationType: model.DownloadNotification, Status: model.StartedNotificationStatus}
	if err := j.RecordPhaseFailure(model.DownloadNotification, "download failed"); err != nil {
		t.Fatal(err)
	}
	client := &fakeProtectedRecoveryClient{leaseErrors: []error{serverclient.ErrProtectedConflict}}
	if err := recoverProtectedTerminal(t.Context(), j, "node", client); !errors.Is(err, serverclient.ErrProtectedConflict) || len(client.events) != 0 {
		t.Fatalf("recovery without lease = %v, events=%+v", err, client.events)
	}
	if _, err := os.Stat(j.WorkingDir); err != nil {
		t.Fatalf("unresolved workspace was removed: %v", err)
	}
}

func TestTerminalFailureIntentMustMatchPendingEvent(t *testing.T) {
	for _, matching := range []bool{true, false} {
		t.Run(fmt.Sprintf("matching=%t", matching), func(t *testing.T) {
			id := uuid.New()
			j := job.NewContext(id, 5, t.TempDir())
			j.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
			j.RequestID = uuid.New()
			j.NextPhase = model.DownloadNotification
			j.LastEvent = &job.ContextEvent{EventId: 6, NotificationType: model.DownloadNotification, Status: model.FailedNotificationStatus, Message: "download failed"}
			j.PhaseFailure = &job.PhaseFailureIntent{Phase: model.DownloadNotification, Message: "download failed"}
			message := "Download:different failure"
			if matching {
				message = "Download:download failed"
			}
			event, err := j.NewProtectedEvent("node", model.JobNotification, model.FailedNotificationStatus, message)
			if err != nil {
				t.Fatal(err)
			}
			if err := j.StageProtectedEvent(event); err != nil {
				t.Fatal(err)
			}
			if got := recoverableTerminalAssignment(j); got != matching {
				t.Fatalf("recoverable = %t, want %t", got, matching)
			}
			if err := recoverProtectedTerminal(t.Context(), j, "node", &fakeProtectedRecoveryClient{}); (err == nil) != matching {
				t.Fatalf("reconcile = %v, matching=%t", err, matching)
			}
		})
	}
}

func TestTransientProtectedErrorsDoNotBecomeTerminalFailures(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, ErrProtectedLeaseLost, ErrProtectedEventUncertain, serverclient.ErrProtectedConflict, serverclient.ErrProtectedUnavailable} {
		if isDefinitiveProtectedFailure(fmt.Errorf("wrapped: %w", err)) {
			t.Fatalf("transient error %v was classified as definitive", err)
		}
	}
	if !isDefinitiveProtectedFailure(errors.New("encoder failed")) {
		t.Fatal("encoder failure was not classified as definitive")
	}
}

func TestProtectedActionPersistsOnlyDefinitiveFailureIntent(t *testing.T) {
	for _, tt := range []struct {
		name       string
		cause      error
		wantIntent bool
	}{
		{name: "encoder error", cause: errors.New("encoder failed"), wantIntent: true},
		{name: "canceled", cause: context.Canceled},
		{name: "server unavailable", cause: serverclient.ErrProtectedUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			j := job.NewContext(id, 5, t.TempDir())
			j.Assignment = &model.AssignmentClaim{JobID: id, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
			j.RequestID = uuid.New()
			j.NextPhase = model.FFMPEGSNotification
			guard, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) { return time.Now().Add(time.Minute), nil }, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer guard.Close()
			client := &fakeProtectedEventClient{}
			e := &JobExecutor{console: console.NewRenderService(), protectedJobs: map[uuid.UUID]*protectedExecution{id: {job: j, guard: guard, events: &protectedEventPublisher{client: client, workerName: "node"}}}}
			executor := step.NewStepExecutor(model.FFMPEGSNotification, func(*job.Context) []step.Action {
				return []step.Action{{Id: id.String(), Execute: func(context.Context, step.Tracker) error { return tt.cause }}}
			})
			if err := e.executeStepActions(guard.ctx, executor, j); !errors.Is(err, tt.cause) {
				t.Fatalf("execute action = %v, want %v", err, tt.cause)
			}
			reloaded, err := job.ReadContextFromDiskByPath(filepath.Join(j.WorkingDir, id.String()+".json"))
			if err != nil || (reloaded.PhaseFailure != nil) != tt.wantIntent {
				t.Fatalf("persisted failure intent = %+v, %v", reloaded, err)
			}
			if tt.wantIntent && !recoverablePhaseFailure(reloaded) {
				t.Fatal("definitive failure checkpoint cannot be recovered")
			}
		})
	}
}

func (f *fakeProtectedRecoveryClient) AcquireLease(_ context.Context, claim model.AssignmentClaim) (time.Time, error) {
	f.leases = append(f.leases, claim)
	if len(f.leaseErrors) > 0 {
		err := f.leaseErrors[0]
		f.leaseErrors = f.leaseErrors[1:]
		if err != nil {
			return time.Time{}, err
		}
	}
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

func TestRecoverPrefetchedCheckpointBeforeParkIntent(t *testing.T) {
	j := testParkableContext(t)
	if j.Parked || j.ParkIntent {
		t.Fatal("fixture already has a park acknowledgement")
	}
	client := &fakeProtectedRecoveryClient{}
	guard, err := recoverParkedJob(t.Context(), j, "node", client, true, time.Millisecond)
	if err != nil || guard != nil || !j.Parked || len(client.leases) != 1 || len(client.parks) != 1 {
		t.Fatalf("recover pre-park checkpoint: guard=%v parked=%t leases=%d parks=%d error=%v", guard, j.Parked, len(client.leases), len(client.parks), err)
	}
}

func TestRecoverPrefetchWaitsForPreviousProcessLease(t *testing.T) {
	j := testParkableContext(t)
	client := &fakeProtectedRecoveryClient{leaseErrors: []error{serverclient.ErrProtectedConflict, nil}}
	if _, err := recoverParkedJob(t.Context(), j, "node", client, true, time.Millisecond); !errors.Is(err, serverclient.ErrProtectedConflict) || j.Parked || len(client.parks) != 0 {
		t.Fatalf("lease conflict changed checkpoint: parked=%t error=%v", j.Parked, err)
	}
	if _, err := recoverParkedJob(t.Context(), j, "node", client, true, time.Millisecond); err != nil || !j.Parked || len(client.parks) != 1 {
		t.Fatalf("recovery after lease expiry: parked=%t error=%v", j.Parked, err)
	}
}

func TestRecoverCompletedPrefetchBeforeNextPhaseWasPersisted(t *testing.T) {
	for _, phase := range []model.NotificationType{model.DownloadNotification, model.MKVExtractNotification, model.PGSNotification} {
		for _, pending := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/pending=%t", phase, pending), func(t *testing.T) {
				j := testParkableContext(t)
				j.NextPhase = phase
				j.LastEvent.NotificationType = phase
				if pending {
					j.LastEvent.Status = model.StartedNotificationStatus
					event, err := j.NewProtectedEvent("node", phase, model.CompletedNotificationStatus, "")
					if err != nil {
						t.Fatal(err)
					}
					if err := j.StageProtectedEvent(event); err != nil {
						t.Fatal(err)
					}
				} else if err := j.PersistJobContext(); err != nil {
					t.Fatal(err)
				}
				reloaded, err := job.ReadContextFromDiskByPath(filepath.Join(j.WorkingDir, j.JobId.String()+".json"))
				if err != nil {
					t.Fatal(err)
				}
				client := &fakeProtectedRecoveryClient{}
				guard, err := recoverParkedJob(t.Context(), reloaded, "node", client, true, time.Millisecond)
				if err != nil || guard != nil || !reloaded.Parked || reloaded.NextPhase != model.FFMPEGSNotification || len(client.parks) != 1 || len(client.events) != boolInt(pending) {
					t.Fatalf("recovery = %v, guard=%v, parked=%t, next=%s, parks=%d, events=%d", err, guard, reloaded.Parked, reloaded.NextPhase, len(client.parks), len(client.events))
				}
				reloaded, err = job.ReadContextFromDiskByPath(filepath.Join(j.WorkingDir, j.JobId.String()+".json"))
				if err != nil || !reloaded.Parked || reloaded.NextPhase != model.FFMPEGSNotification || reloaded.PendingProtectedEvent() != nil {
					t.Fatalf("persisted recovery = %+v, %v", reloaded, err)
				}
			})
		}
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestRecoverCompletedPrefetchDoesNotSkipSubtitlePreparation(t *testing.T) {
	for _, phase := range []model.NotificationType{model.DownloadNotification, model.MKVExtractNotification} {
		t.Run(string(phase), func(t *testing.T) {
			j := testParkableContext(t)
			j.NextPhase = phase
			j.LastEvent.NotificationType = phase
			j.Source.FFProbeData.Subtitle = []*ffmpeg.Subtitle{{Format: "hdmv_pgs_subtitle"}}
			client := &fakeProtectedRecoveryClient{}
			if guard, err := recoverParkedJob(t.Context(), j, "node", client, true, time.Millisecond); err == nil || guard != nil || len(client.leases) != 0 {
				t.Fatalf("unfinished subtitle preparation was recovered: guard=%v error=%v leases=%d", guard, err, len(client.leases))
			}
		})
	}
}
