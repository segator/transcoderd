package job

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"transcoder/model"
	"transcoder/worker/ffmpeg"

	"github.com/google/uuid"
)

func TestNewContext(t *testing.T) {
	jobId := uuid.New()
	eventId := 1
	workingDir := "/tmp/test"

	ctx := NewContext(jobId, eventId, workingDir)

	if ctx.JobId != jobId {
		t.Errorf("JobId = %v, want %v", ctx.JobId, jobId)
	}
	if ctx.EventId != eventId {
		t.Errorf("EventId = %d, want %d", ctx.EventId, eventId)
	}
	if ctx.WorkingDir != workingDir {
		t.Errorf("WorkingDir = %s, want %s", ctx.WorkingDir, workingDir)
	}
	if ctx.Source == nil {
		t.Error("Source should not be nil")
	}
}

func TestValidatePrefetchedSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	data := []byte("prefetched source")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	base := VideoData{FilePath: path, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:]), FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{}}}
	tests := []struct {
		name   string
		source *VideoData
		wantOK bool
	}{
		{"valid", &base, true},
		{"missing metadata", nil, false},
		{"missing video metadata", &VideoData{FilePath: path, Size: base.Size, Checksum: base.Checksum, FFProbeData: &ffmpeg.NormalizedFFProbe{}}, false},
		{"wrong checksum", &VideoData{FilePath: path, Size: int64(len(data)), Checksum: "bad", FFProbeData: base.FFProbeData}, false},
		{"wrong size", &VideoData{FilePath: path, Size: int64(len(data)) + 1, Checksum: base.Checksum, FFProbeData: base.FFProbeData}, false},
		{"missing file", &VideoData{FilePath: filepath.Join(dir, "missing.mkv"), Size: base.Size, Checksum: base.Checksum, FFProbeData: base.FFProbeData}, false},
		{"outside workspace", &VideoData{FilePath: filepath.Join(t.TempDir(), "outside.mkv"), Size: base.Size, Checksum: base.Checksum, FFProbeData: base.FFProbeData}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{WorkingDir: dir, Source: tt.source}
			err := ctx.ValidatePrefetchedSource()
			if tt.wantOK && err != nil {
				t.Errorf("ValidatePrefetchedSource() = %v, want nil", err)
			}
			if !tt.wantOK && !errors.Is(err, ErrInvalidPrefetchedSource) {
				t.Errorf("ValidatePrefetchedSource() = %v, want ErrInvalidPrefetchedSource", err)
			}
		})
	}
}

func TestContextUpdateEvent(t *testing.T) {
	ctx := NewContext(uuid.New(), 1, "/tmp/test")

	notifType := model.DownloadNotification
	status := model.StartedNotificationStatus
	message := "Download started"

	ctx.UpdateEvent(notifType, status, message)

	if ctx.LastEvent == nil {
		t.Fatal("LastEvent should not be nil")
	}
	if ctx.LastEvent.NotificationType != notifType {
		t.Errorf("NotificationType = %v, want %v", ctx.LastEvent.NotificationType, notifType)
	}
	if ctx.LastEvent.Status != status {
		t.Errorf("Status = %v, want %v", ctx.LastEvent.Status, status)
	}
	if ctx.LastEvent.Message != message {
		t.Errorf("Message = %s, want %s", ctx.LastEvent.Message, message)
	}
	if ctx.LastEvent.EventId != 2 {
		t.Errorf("EventId = %d, want %d", ctx.LastEvent.EventId, 2)
	}
}

func TestContextUpdateEventMultipleTimes(t *testing.T) {
	ctx := NewContext(uuid.New(), 1, "/tmp/test")

	ctx.UpdateEvent(model.DownloadNotification, model.StartedNotificationStatus, "First")
	firstEventId := ctx.LastEvent.EventId

	ctx.UpdateEvent(model.UploadNotification, model.CompletedNotificationStatus, "Second")
	secondEventId := ctx.LastEvent.EventId

	if secondEventId != firstEventId+1 {
		t.Errorf("Second EventId = %d, want %d", secondEventId, firstEventId+1)
	}
}

func TestContextInitAndClean(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "transcoder_test_"+uuid.New().String())
	ctx := NewContext(uuid.New(), 1, tmpDir)

	// Test Init creates directory
	err := ctx.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("Init() should create working directory")
	}

	// Test Clean removes directory
	err = ctx.Clean()
	if err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("Clean() should remove working directory")
	}
}

func TestContextPersistJobContext(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "transcoder_test_"+uuid.New().String())
	jobId := uuid.New()
	ctx := NewContext(jobId, 1, tmpDir)

	err := ctx.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		if err := ctx.Clean(); err != nil {
			t.Errorf("Clean() error = %v", err)
		}
	}()

	ctx.UpdateEvent(model.DownloadNotification, model.CompletedNotificationStatus, "Test message")

	err = ctx.PersistJobContext()
	if err != nil {
		t.Fatalf("PersistJobContext() error = %v", err)
	}

	// Verify file was created
	contextFile := filepath.Join(tmpDir, jobId.String()+".json")
	if _, err := os.Stat(contextFile); os.IsNotExist(err) {
		t.Error("PersistJobContext() should create context file")
	}

	// Verify file content
	data, err := os.ReadFile(contextFile)
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}

	var loadedCtx Context
	err = json.Unmarshal(data, &loadedCtx)
	if err != nil {
		t.Fatalf("Failed to unmarshal context: %v", err)
	}

	if loadedCtx.JobId != ctx.JobId {
		t.Errorf("Loaded JobId = %v, want %v", loadedCtx.JobId, ctx.JobId)
	}

	ctx.NextPhase = model.FFMPEGSNotification
	if err := ctx.PersistJobContext(); err != nil {
		t.Fatalf("PersistJobContext() replacement error = %v", err)
	}
	reloaded, err := ReadContextFromDiskByPath(contextFile)
	if err != nil || reloaded.NextPhase != model.FFMPEGSNotification {
		t.Fatalf("ReadContextFromDiskByPath() = %+v, %v, want FFmpeg next phase", reloaded, err)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("context directory contains %d entries, err = %v; want one snapshot", len(entries), err)
	}
}
