package step

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type fakeProtectedTransfers struct {
	data      []byte
	checksum  string
	claim     model.AssignmentClaim
	uploaded  []byte
	uploadErr error
}

func (f *fakeProtectedTransfers) Download(_ context.Context, claim model.AssignmentClaim) (*serverclient.ProtectedDownload, error) {
	f.claim = claim
	return &serverclient.ProtectedDownload{Body: io.NopCloser(bytes.NewReader(f.data)), Name: "film.mkv", Size: int64(len(f.data))}, nil
}

func (f *fakeProtectedTransfers) Checksum(context.Context, model.AssignmentClaim) (string, error) {
	return f.checksum, nil
}

func (f *fakeProtectedTransfers) Upload(_ context.Context, claim model.AssignmentClaim, source io.Reader, _ int64, _ string) error {
	f.claim = claim
	if f.uploadErr != nil {
		return f.uploadErr
	}
	data, err := io.ReadAll(source)
	f.uploaded = data
	return err
}

func TestProtectedDownloadUsesClaimAndValidatesSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test uses a shell ffprobe fixture")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '{\"format\":{\"duration\":\"1\"},\"streams\":[{\"index\":0,\"codec_type\":\"video\",\"codec_name\":\"h264\",\"avg_frame_rate\":\"24/1\",\"width\":16,\"height\":16}]}'\n"
	if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	data := []byte("downloaded bytes")
	sum := sha256.Sum256(data)
	client := &fakeProtectedTransfers{data: data, checksum: hex.EncodeToString(sum[:])}
	j := job.NewContext(uuid.New(), 1, t.TempDir())
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	executor := &DownloadStepExecutor{protected: client}
	got, err := executor.download(t.Context(), &mockStepTracker{}, j)
	if err != nil || got == nil || got.Checksum != client.checksum || got.FFProbeData == nil || got.FFProbeData.Video == nil || client.claim != *j.Assignment {
		t.Fatalf("protected download = %+v, %v", got, err)
	}
	j.Source = got
	if err := j.ValidatePrefetchedSource(); err != nil {
		t.Fatalf("downloaded source is not recoverable: %v", err)
	}
}

func TestProtectedUploadNeverFallsBackOrRetriesStaleAssignment(t *testing.T) {
	data := []byte("encoded bytes")
	path := filepath.Join(t.TempDir(), "output.mkv")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	j := job.NewContext(uuid.New(), 1, t.TempDir())
	j.Assignment = &model.AssignmentClaim{JobID: j.JobId, Generation: 1, Capability: "secret", InstanceID: uuid.New()}
	j.Target = &job.VideoData{FilePath: path, Checksum: hex.EncodeToString(sum[:])}
	if err := (&UploadStepExecutor{}).upload(t.Context(), &mockStepTracker{}, j); err == nil {
		t.Fatal("protected job fell back to legacy upload")
	}
	client := &fakeProtectedTransfers{}
	executor := &UploadStepExecutor{protected: client}
	if err := executor.upload(t.Context(), &mockStepTracker{}, j); err != nil || !bytes.Equal(client.uploaded, data) || client.claim != *j.Assignment {
		t.Fatalf("protected upload = %q, %v", client.uploaded, err)
	}
	client.uploadErr = serverclient.ErrProtectedConflict
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := executor.upload(ctx, &mockStepTracker{}, j); !errors.Is(err, serverclient.ErrProtectedConflict) {
		t.Fatalf("stale protected upload = %v", err)
	}
}
