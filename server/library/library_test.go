package library

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"

	"github.com/google/uuid"
)

type mediaBackend struct {
	*repository.InMemoryRepository
	root    string
	minimum int64
	fail    string
	block   bool
	mu      sync.Mutex
	calls   []string
}

func (b *mediaBackend) MediaSourceRoot() string { return b.root }
func (b *mediaBackend) MediaMinimumSize() int64 { return b.minimum }
func (b *mediaBackend) QueueMediaFile(ctx context.Context, f model.MediaFile, _ model.MediaFilters) (*model.Job, error) {
	if b.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	b.mu.Lock()
	b.calls = append(b.calls, f.Path)
	b.mu.Unlock()
	if f.Path == b.fail {
		return nil, errors.New("injected queue failure")
	}
	j := &model.Job{Id: uuid.New(), SourcePath: f.Path, TargetPath: f.Path + "_encoded.mkv"}
	if err := b.AddJob(ctx, j); err != nil {
		return nil, err
	}
	if err := b.AddNewTaskEvent(ctx, j.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
		return nil, err
	}
	return j, nil
}
func newTestLibrary(t *testing.T) (*Library, *mediaBackend) {
	t.Helper()
	b := &mediaBackend{InMemoryRepository: repository.NewInMemoryRepository(), root: t.TempDir(), minimum: 10}
	l := New(b, map[string]string{"encoded": t.TempDir()}, 1)
	t.Cleanup(l.Close)
	return l, b
}
func seedVideo(t *testing.T, l *Library, p string, size int) model.MediaFile {
	t.Helper()
	full := filepath.Join(l.backend.MediaSourceRoot(), p)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, bytes.Repeat([]byte("x"), size), 0600); err != nil {
		t.Fatal(err)
	}
	f, entry, err := l.Open(FileID("source", p))
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	r, key, err := l.openRoot("source")
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	probe := &model.MediaProbe{DurationSeconds: 0.00001, Streams: []model.MediaStream{{CodecType: "video", CodecName: "h264", Width: 1920, Height: 1080}}}
	if err := l.backend.PutMediaProbe(context.Background(), key, p, entry.Size, entry.ModifiedNS, probe); err != nil {
		t.Fatal(err)
	}
	return entry
}
func awaitOperation(t *testing.T, l *Library, id string) model.MediaOperation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		op, err := l.Operation(id)
		if err != nil {
			t.Fatal(err)
		}
		if op.State != "running" {
			return op
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("media operation did not finish")
	return model.MediaOperation{}
}
func TestRootedBrowsingAndHiddenPaths(t *testing.T) {
	l, b := newTestLibrary(t)
	seedVideo(t, l, "Peliculas/visible.mkv", 100)
	if err := os.MkdirAll(filepath.Join(b.root, ".transcoderd"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.root, ".transcoderd/hidden.mkv"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(b.root, "Peliculas"), filepath.Join(b.root, "link")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.mkv")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(b.root, "escape.mkv")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../secret.mkv", "/etc/secret.mkv", ".transcoderd/hidden.mkv", "link/visible.mkv", "escape.mkv", "Peliculas/../visible.mkv", "Peliculas\\visible.mkv"} {
		f, _, err := l.Open(FileID("source", p))
		if err == nil {
			_ = f.Close()
			t.Errorf("Open(%q) succeeded", p)
		}
	}
	result, err := l.Browse(context.Background(), "source", ".", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "Peliculas" {
		t.Fatalf("Browse() = %+v, want only visible folder", result)
	}
	if _, err := l.StartScan(model.MediaFilters{Path: "Peliculas/visible.mkv"}); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("scan regular file = %v", err)
	}
	entries := []model.MediaFile{{Root: "source", Path: "tiny.mkv", Size: 1}}
	j := &model.Job{Id: uuid.New(), SourcePath: "tiny.mkv"}
	_ = b.AddJob(context.Background(), j)
	_ = b.AddNewTaskEvent(context.Background(), j.AddEvent(model.JobNotification, model.FailedNotificationStatus))
	if err := l.annotate(context.Background(), entries, model.MediaFilters{RetryFailed: true}); err != nil {
		t.Fatal(err)
	}
	if entries[0].Eligible {
		t.Error("retry bypassed server minimum size")
	}
}
func TestScriptPresetsAndStrictFilters(t *testing.T) {
	presets := Presets()
	tests := []struct {
		name string
		f    model.MediaFilters
		m    model.MediaMetadata
		want bool
	}{
		{"width boundary", presets["movies"], model.MediaMetadata{Width: 1800, Codec: "h264", BitrateMbps: 5}, false},
		{"bitrate boundary", presets["movies"], model.MediaMetadata{Width: 1920, Codec: "h264", BitrateMbps: 4}, false},
		{"strictly above", presets["movies"], model.MediaMetadata{Width: 1801, Codec: "h264", BitrateMbps: 4.001}, true},
		{"hevc alias", presets["series"], model.MediaMetadata{Width: 1920, Codec: "x265", BitrateMbps: 5}, false},
		{"movie AV1", presets["movies"], model.MediaMetadata{Width: 1920, Codec: "av1", BitrateMbps: 5}, false},
		{"series AV1", presets["series"], model.MediaMetadata{Width: 1920, Codec: "av1", BitrateMbps: 5}, true},
		{"include alias", model.MediaFilters{IncludeCodecs: []string{"x264"}}, model.MediaMetadata{Codec: "h264"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metadataMatch(&tt.m, tt.f); got != tt.want {
				t.Errorf("metadataMatch() = %v, want %v", got, tt.want)
			}
		})
	}
	if !cheapMatch(model.MediaFile{Path: "peliculas/Example.MKV", Size: 20}, model.MediaFilters{Contains: "PELICULAS", Exclude: "other", MinBytes: 20, MaxBytes: 20}) {
		t.Error("case-insensitive size/path match failed")
	}
}
func TestReviewedBatchSuccessfulTargetAndStaleFiles(t *testing.T) {
	l, b := newTestLibrary(t)
	for i, p := range []string{"Peliculas/a.mkv", "Peliculas/b.mkv", "Peliculas/c.mkv", "Peliculas/d.mkv", "Peliculas/e.mkv"} {
		seedVideo(t, l, p, 500-i*50)
	}
	op, err := l.StartScan(Presets()["movies"])
	if err != nil {
		t.Fatal(err)
	}
	scan := awaitOperation(t, l, op.ID)
	if scan.State != "completed" || len(scan.Files) != 5 {
		t.Fatalf("scan = %+v", scan)
	}
	if len(b.calls) != 0 {
		t.Fatal("preview mutated job queue")
	}
	if scan.Files[0].Path != "Peliculas/a.mkv" {
		t.Fatal("scan not ordered largest first")
	}
	ids := []string{}
	for _, f := range scan.Files {
		ids = append(ids, f.ID)
	}
	if _, err := l.StartBatch(model.MediaBatchRequest{ScanID: scan.ID, IDs: []string{FileID("source", "other.mkv")}, Target: 1}); err == nil {
		t.Fatal("unreviewed selection accepted")
	}
	if err := os.WriteFile(filepath.Join(b.root, "Peliculas/a.mkv"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	// An independently queued job appearing after review must be skipped.
	j := &model.Job{Id: uuid.New(), SourcePath: "Peliculas/b.mkv"}
	_ = b.AddJob(context.Background(), j)
	_ = b.AddNewTaskEvent(context.Background(), j.AddEvent(model.JobNotification, model.QueuedNotificationStatus))
	b.fail = "Peliculas/c.mkv"
	batch, err := l.StartBatch(model.MediaBatchRequest{ScanID: scan.ID, IDs: ids, Target: 1})
	if err != nil {
		t.Fatal(err)
	}
	loaded := awaitOperation(t, l, batch.ID)
	if loaded.Scheduled != 1 || loaded.Skipped != 2 || loaded.Failed != 1 || loaded.Examined != 4 || loaded.State != "completed" {
		t.Fatalf("batch = %+v", loaded)
	}
	if got := b.calls; len(got) != 2 || got[1] != "Peliculas/d.mkv" {
		t.Errorf("queued calls = %v", got)
	}
	// Limits are explicit; the scan never silently claims the entire folder.
	capped, err := l.StartScan(model.MediaFilters{Recursive: true, CandidateLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := awaitOperation(t, l, capped.ID)
	if !got.Truncated || got.Candidates != 1 {
		t.Fatalf("capped scan = %+v", got)
	}
}
func TestOperationCancellationAndExpiry(t *testing.T) {
	l, b := newTestLibrary(t)
	seedVideo(t, l, "one.mkv", 100)
	scan, err := l.StartScan(Presets()["all"])
	if err != nil {
		t.Fatal(err)
	}
	scan = awaitOperation(t, l, scan.ID)
	b.block = true
	batch, err := l.StartBatch(model.MediaBatchRequest{ScanID: scan.ID, IDs: []string{scan.Files[0].ID}, Target: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Cancel(batch.ID); err != nil {
		t.Fatal(err)
	}
	got := awaitOperation(t, l, batch.ID)
	if got.State != "canceled" || got.Scheduled != 0 {
		t.Fatalf("canceled batch = %+v", got)
	}
	l.mu.Lock()
	l.operations[scan.ID].data.UpdatedAt = time.Now().Add(-31 * time.Minute)
	l.mu.Unlock()
	if _, err := l.Operation(scan.ID); !errors.Is(err, ErrOperationNotFound) {
		t.Fatalf("expired operation = %v", err)
	}
}
func TestRealMetadataCachePreviewSeekAndCancellation(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	l, b := newTestLibrary(t)
	p := filepath.Join(b.root, "clip.mkv")
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440", "-t", "6", "-c:v", "libx264", "-threads", "1", "-c:a", "aac", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate video: %v %s", err, out)
	}
	id := FileID("source", "clip.mkv")
	info, err := l.Info(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if info.Metadata.Width != 320 || info.Metadata.DurationSeconds < 6 || info.Metadata.Codec != "h264" {
		t.Fatalf("real metadata = %+v", info.Metadata)
	}
	// Cache must work with ffprobe removed from PATH, and invalidate on mtime.
	processPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	if _, err := l.Info(context.Background(), id); err != nil {
		t.Fatalf("cached metadata = %v", err)
	}
	if err := os.Chtimes(p, time.Now(), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Info(context.Background(), id); err == nil {
		t.Fatal("changed fingerprint incorrectly reused cached probe")
	}
	// Restore process PATH for the genuine preview and independently decode it.
	t.Setenv("PATH", processPath)
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := l.Preview(ctx, id, 4, &out); err != nil {
		t.Fatal(err)
	}
	mp4 := filepath.Join(t.TempDir(), "preview.mp4")
	if err := os.WriteFile(mp4, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	decode := exec.Command("ffmpeg", "-v", "error", "-i", mp4, "-f", "null", "-")
	if result, err := decode.CombinedOutput(); err != nil {
		t.Fatalf("decode preview: %v %s", err, result)
	}
	previewCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Preview(previewCtx, id, 0, io.Discard) }()
	deadline := time.Now().Add(time.Second)
	for len(l.streams) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := l.Preview(context.Background(), id, 0, io.Discard); !errors.Is(err, ErrBusy) {
		t.Errorf("stream limit = %v", err)
	}
	stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("preview process did not stop on disconnect")
	}
	if len(l.streams) != 0 {
		t.Fatal("preview slot leaked")
	}
}

func TestJobAssetsCompletedOutputAfterSourceRemoval(t *testing.T) {
	l, b := newTestLibrary(t)
	ctx := context.Background()
	source := seedVideo(t, l, "source.mkv", 100)
	outputPath := filepath.Join(l.roots["encoded"], "output.mp4")
	if err := os.WriteFile(outputPath, []byte("output"), 0600); err != nil {
		t.Fatal(err)
	}
	j := &model.Job{Id: uuid.New(), SourcePath: source.Path, TargetPath: "output.mp4"}
	if err := b.AddJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	for _, event := range []*model.TaskEventType{j.AddEvent(model.JobNotification, model.StartedNotificationStatus), j.AddEvent(model.FFMPEGSNotification, model.CompletedNotificationStatus)} {
		if err := b.AddNewTaskEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := l.JobAssets(ctx, j.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Job.Role != "source" {
		t.Fatalf("intermediate completion assets = %+v", assets)
	}
	if err := b.AddNewTaskEvent(ctx, j.AddEvent(model.JobNotification, model.CompletedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(b.root, source.Path)); err != nil {
		t.Fatal(err)
	}
	assets, err = l.JobAssets(ctx, j.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Root != "encoded" || assets[0].Job.Role != "output" {
		t.Fatalf("completed output with removed source = %+v", assets)
	}
}

func TestOperationRegistryBounds(t *testing.T) {
	l, _ := newTestLibrary(t)
	a, _, err := l.newOperation("scan")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := l.newOperation("scan")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.newOperation("scan"); !errors.Is(err, ErrBusy) {
		t.Fatalf("third active operation = %v", err)
	}
	l.finish(a, nil)
	l.finish(b, nil)
	for i := 0; i < 10; i++ {
		op, _, err := l.newOperation("scan")
		if err != nil {
			t.Fatal(err)
		}
		l.finish(op, nil)
	}
	if len(l.operations) != 8 {
		t.Errorf("registry count = %d, want 8", len(l.operations))
	}
}

type cancelAfterFirstBackend struct {
	*mediaBackend
	started chan struct{}
}

func (b *cancelAfterFirstBackend) QueueMediaFile(ctx context.Context, f model.MediaFile, filters model.MediaFilters) (*model.Job, error) {
	if f.Path == "second.mkv" {
		close(b.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.mediaBackend.QueueMediaFile(ctx, f, filters)
}
func TestBatchCancellationKeepsPreviouslyQueuedJobs(t *testing.T) {
	l, b := newTestLibrary(t)
	seedVideo(t, l, "first.mkv", 200)
	seedVideo(t, l, "second.mkv", 100)
	blocking := &cancelAfterFirstBackend{mediaBackend: b, started: make(chan struct{})}
	l.backend = blocking
	scan, err := l.StartScan(Presets()["all"])
	if err != nil {
		t.Fatal(err)
	}
	scan = awaitOperation(t, l, scan.ID)
	batch, err := l.StartBatch(model.MediaBatchRequest{ScanID: scan.ID, IDs: []string{scan.Files[0].ID, scan.Files[1].ID}, Target: 2})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("batch did not reach second file")
	}
	if err := l.Cancel(batch.ID); err != nil {
		t.Fatal(err)
	}
	got := awaitOperation(t, l, batch.ID)
	if got.State != "canceled" || got.Scheduled != 1 {
		t.Fatalf("partially canceled batch = %+v", got)
	}
	j, err := b.GetJobByPath(context.Background(), "first.mkv")
	if err != nil || j == nil {
		t.Fatalf("previously committed job missing: %+v %v", j, err)
	}
}
