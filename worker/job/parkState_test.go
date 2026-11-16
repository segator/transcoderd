package job

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"transcoder/model"
	"transcoder/worker/ffmpeg"

	"github.com/google/uuid"
)

func TestParkIntentAndResumeAreDurable(t *testing.T) {
	workspace := t.TempDir()
	j := NewContext(uuid.New(), 3, workspace)
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	j.NextPhase = model.FFMPEGSNotification
	j.LastEvent = &ContextEvent{EventId: 5, NotificationType: model.DownloadNotification, Status: model.CompletedNotificationStatus}
	source := []byte("prefetched")
	path := filepath.Join(workspace, "source.mkv")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(source)
	j.Source = &VideoData{FilePath: path, Size: int64(len(source)), Checksum: hex.EncodeToString(sum[:]), FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{}}}
	if err := j.ConfirmPark(); err == nil {
		t.Fatal("park acknowledgement without an intent succeeded")
	}
	if err := j.PreparePark(); err != nil {
		t.Fatal(err)
	}
	contextPath := filepath.Join(workspace, j.JobId.String()+".json")
	reloaded, err := ReadContextFromDiskByPath(contextPath)
	if err != nil || !reloaded.ParkIntent || reloaded.Parked {
		t.Fatalf("park intent after restart = %+v, %v", reloaded, err)
	}
	if err := reloaded.ConfirmPark(); err != nil {
		t.Fatal(err)
	}
	reloaded, err = ReadContextFromDiskByPath(contextPath)
	if err != nil || reloaded.ParkIntent || !reloaded.Parked {
		t.Fatalf("park acknowledgement after restart = %+v, %v", reloaded, err)
	}
	instance := uuid.New()
	if err := reloaded.RecordAcquiredLease(instance); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.ResumeWithLease(instance); err != nil {
		t.Fatal(err)
	}
	reloaded, err = ReadContextFromDiskByPath(contextPath)
	if err != nil || reloaded.Parked || reloaded.ParkIntent || reloaded.Assignment.InstanceID != instance || reloaded.NextPhase != model.FFMPEGSNotification {
		t.Fatalf("resumed assignment after restart = %+v, %v", reloaded, err)
	}
}

func TestPrepareParkRejectsUnusablePrefetch(t *testing.T) {
	j := NewContext(uuid.New(), 3, t.TempDir())
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	j.NextPhase = model.FFMPEGSNotification
	j.LastEvent = &ContextEvent{EventId: 5, NotificationType: model.DownloadNotification, Status: model.CompletedNotificationStatus}
	if err := j.PreparePark(); err == nil || j.ParkIntent || j.Parked {
		t.Fatalf("PreparePark() with missing source = %v, intent=%t parked=%t", err, j.ParkIntent, j.Parked)
	}
}
