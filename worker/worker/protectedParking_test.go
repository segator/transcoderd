package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type fakeProtectedParkClient struct {
	errors []error
	calls  int
}

func TestParkedEncodeWaitsForLeaseAndRestartBarrier(t *testing.T) {
	j := testParkableContext(t)
	executor := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: 1}, maxActiveJobs: 1, encodePool: newEncodeConcurrency(1)}
	guard, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) {
		return time.Now().Add(time.Minute), nil
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeProtectedRecoveryClient{}
	if err := executor.parkPrefetched(t.Context(), j, guard, client, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, ok := executor.encodePool.tryStartEncoder("", j.JobId); ok {
		t.Fatal("parked encode started before reclaiming its lease")
	}
	guard, err = recoverParkedJob(t.Context(), j, "node", client, false, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	executor.BeginRestartBarrier()
	if err := executor.releaseParkedEncode(j, guard); err == nil {
		t.Fatal("queue resume bypassed the restart barrier")
	}
	executor.ReleaseRestartBarrier()
	if err := executor.releaseParkedEncode(j, guard); err != nil {
		t.Fatal(err)
	}
	kind, ok := executor.encodePool.tryStartEncoder("", j.JobId)
	if !ok {
		t.Fatal("reclaimed prefetched encode did not start")
	}
	executor.encodePool.finishEncoder(kind, j.JobId)
}

func (f *fakeProtectedParkClient) Park(_ context.Context, _ model.AssignmentClaim) error {
	f.calls++
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}

func testParkableContext(t *testing.T) *job.Context {
	t.Helper()
	workspace := t.TempDir()
	j := job.NewContext(uuid.New(), 1, workspace)
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	j.NextPhase = model.FFMPEGSNotification
	j.LastEvent = &job.ContextEvent{EventId: 3, NotificationType: model.DownloadNotification, Status: model.CompletedNotificationStatus}
	data := []byte("prefetch")
	path := filepath.Join(workspace, "source.mkv")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	j.Source = &job.VideoData{FilePath: path, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:]), FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{}}}
	return j
}

func TestProtectedParkingRetriesLostResponseAfterStoppingRenewal(t *testing.T) {
	j := testParkableContext(t)
	guard, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) {
		return time.Now().Add(time.Minute), nil
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeProtectedParkClient{errors: []error{errors.New("response lost"), nil}}
	if err := parkProtectedJob(t.Context(), j, guard, client, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if guard.ctx.Err() == nil || client.calls != 2 || !j.Parked || j.ParkIntent {
		t.Fatalf("park state: lease canceled=%t calls=%d parked=%t intent=%t", guard.ctx.Err() != nil, client.calls, j.Parked, j.ParkIntent)
	}
}

func TestProtectedParkingConflictNeverAcknowledgesIdle(t *testing.T) {
	j := testParkableContext(t)
	guard, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) {
		return time.Now().Add(time.Minute), nil
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeProtectedParkClient{errors: []error{serverclient.ErrProtectedConflict}}
	if err := parkProtectedJob(t.Context(), j, guard, client, time.Millisecond); !errors.Is(err, serverclient.ErrProtectedConflict) || j.Parked || !j.ParkIntent {
		t.Fatalf("park conflict = %v; parked=%t intent=%t", err, j.Parked, j.ParkIntent)
	}
}
