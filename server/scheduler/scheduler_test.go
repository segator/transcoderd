package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"transcoder/model"
	"transcoder/server/repository"

	"github.com/google/uuid"
)

// createTestScheduler sets up a RuntimeScheduler with an in-memory repository
// and a temp directory as the source path.
func createTestScheduler(t *testing.T) (*RuntimeScheduler, *repository.InMemoryRepository, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "scheduler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	repo := repository.NewInMemoryRepository()
	config := &Config{
		ScheduleTime: 5 * time.Minute,
		JobTimeout:   30 * time.Minute,
		SourcePath:   tmpDir,
		MinFileSize:  0,
	}
	sched, err := NewScheduler(config, repo)
	if err != nil {
		t.Fatalf("NewScheduler() error: %v", err)
	}
	return sched, repo, tmpDir
}

// addCompletedJob creates a job in the repository and transitions it to completed status.
func addCompletedJob(t *testing.T, repo *repository.InMemoryRepository, sourcePath, targetPath string) *model.Job {
	t.Helper()
	ctx := context.Background()
	id, _ := uuid.NewUUID()
	job := &model.Job{
		Id:         id,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		SourceSize: 1000,
	}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatalf("AddJob() error: %v", err)
	}
	// queued
	ev := job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)
	if err := repo.AddNewTaskEvent(ctx, ev); err != nil {
		t.Fatalf("AddNewTaskEvent() error: %v", err)
	}
	// assigned
	ev = job.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
	ev.WorkerName = "test-worker"
	if err := repo.AddNewTaskEvent(ctx, ev); err != nil {
		t.Fatalf("AddNewTaskEvent() error: %v", err)
	}
	// started
	ev = job.AddEvent(model.JobNotification, model.StartedNotificationStatus)
	ev.WorkerName = "test-worker"
	if err := repo.AddNewTaskEvent(ctx, ev); err != nil {
		t.Fatalf("AddNewTaskEvent() error: %v", err)
	}
	// completed
	ev = job.AddEvent(model.JobNotification, model.CompletedNotificationStatus)
	ev.WorkerName = "test-worker"
	if err := repo.AddNewTaskEvent(ctx, ev); err != nil {
		t.Fatalf("AddNewTaskEvent() error: %v", err)
	}
	return job
}

// createFile creates a file at the given path relative to baseDir, creating
// any intermediate directories as needed.
func createFile(t *testing.T, baseDir, relativePath string) {
	t.Helper()
	fullPath := filepath.Join(baseDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
}

func TestCompletedJobMaintenance_RequeuesWhenTargetMissing(t *testing.T) {
	sched, repo, tmpDir := createTestScheduler(t)
	ctx := context.Background()

	// Create a completed job whose source exists but target does not
	sourcePath := "movies/test.mkv"
	targetPath := "movies/test_encoded.mkv"
	createFile(t, tmpDir, sourcePath)
	addCompletedJob(t, repo, sourcePath, targetPath)

	err := sched.completedJobMaintenance(ctx)
	if err != nil {
		t.Fatalf("completedJobMaintenance() error: %v", err)
	}

	// Job should now be re-queued
	jobs, err := repo.GetJobsByStatus(ctx, model.JobNotification, model.QueuedNotificationStatus)
	if err != nil {
		t.Fatalf("GetJobsByStatus() error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 queued job, got %d", len(jobs))
	}
}

func TestCompletedJobMaintenance_SkipsWhenTargetExists(t *testing.T) {
	sched, repo, tmpDir := createTestScheduler(t)
	ctx := context.Background()

	// Create a completed job where both source and target exist
	sourcePath := "movies/test.mkv"
	targetPath := "movies/test_encoded.mkv"
	createFile(t, tmpDir, sourcePath)
	createFile(t, tmpDir, targetPath)
	addCompletedJob(t, repo, sourcePath, targetPath)

	err := sched.completedJobMaintenance(ctx)
	if err != nil {
		t.Fatalf("completedJobMaintenance() error: %v", err)
	}

	// Job should still be completed, not re-queued
	jobs, err := repo.GetJobsByStatus(ctx, model.JobNotification, model.QueuedNotificationStatus)
	if err != nil {
		t.Fatalf("GetJobsByStatus() error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 queued jobs, got %d", len(jobs))
	}
}

func TestCompletedJobMaintenance_SkipsWhenBothMissing(t *testing.T) {
	sched, repo, _ := createTestScheduler(t)
	ctx := context.Background()

	// Create a completed job where neither source nor target exists
	sourcePath := "movies/deleted.mkv"
	targetPath := "movies/deleted_encoded.mkv"
	addCompletedJob(t, repo, sourcePath, targetPath)

	err := sched.completedJobMaintenance(ctx)
	if err != nil {
		t.Fatalf("completedJobMaintenance() error: %v", err)
	}

	// Job should NOT be re-queued (no source to re-encode from)
	jobs, err := repo.GetJobsByStatus(ctx, model.JobNotification, model.QueuedNotificationStatus)
	if err != nil {
		t.Fatalf("GetJobsByStatus() error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 queued jobs, got %d", len(jobs))
	}
}

func TestCompletedJobMaintenance_NoCompletedJobs(t *testing.T) {
	sched, _, _ := createTestScheduler(t)
	ctx := context.Background()

	// No jobs at all — should not error
	err := sched.completedJobMaintenance(ctx)
	if err != nil {
		t.Fatalf("completedJobMaintenance() error: %v", err)
	}
}

func TestCompletedJobMaintenance_MultipleJobs(t *testing.T) {
	sched, repo, tmpDir := createTestScheduler(t)
	ctx := context.Background()

	// Job 1: target missing, source exists → should re-queue
	createFile(t, tmpDir, "movies/a.mkv")
	addCompletedJob(t, repo, "movies/a.mkv", "movies/a_encoded.mkv")

	// Job 2: target exists → should NOT re-queue
	createFile(t, tmpDir, "movies/b.mkv")
	createFile(t, tmpDir, "movies/b_encoded.mkv")
	addCompletedJob(t, repo, "movies/b.mkv", "movies/b_encoded.mkv")

	// Job 3: both missing → should NOT re-queue
	addCompletedJob(t, repo, "movies/c.mkv", "movies/c_encoded.mkv")

	err := sched.completedJobMaintenance(ctx)
	if err != nil {
		t.Fatalf("completedJobMaintenance() error: %v", err)
	}

	jobs, err := repo.GetJobsByStatus(ctx, model.JobNotification, model.QueuedNotificationStatus)
	if err != nil {
		t.Fatalf("GetJobsByStatus() error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 queued job, got %d", len(jobs))
	}
	if jobs[0].SourcePath != "movies/a.mkv" {
		t.Errorf("expected re-queued job source 'movies/a.mkv', got %q", jobs[0].SourcePath)
	}
}

func TestVerifyFailureMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "mov_text subtitle now retries",
			message: "error: Subtitle: mov_text is not supported",
			want:    true,
		},
		{
			name:    "WebVTT codec now retries",
			message: "unsupported AVCodecID S_TEXT/WEBVTT in stream 3",
			want:    true,
		},
		{
			name:    "disk quota exceeded retries",
			message: "Disk quota exceeded",
			want:    true,
		},
		{
			name:    "no space left retries",
			message: "No space left on device",
			want:    true,
		},
		{
			name:    "job not found does not retry",
			message: "job not found",
			want:    false,
		},
		{
			name:    "index out of range does not retry",
			message: "runtime error: index out of range [3] with length 2",
			want:    false,
		},
		{
			name:    "source file size does not retry",
			message: "source File size mismatch",
			want:    false,
		},
		{
			name:    "404 download does not retry",
			message: "not 200 respose in dowload code 404",
			want:    false,
		},
		{
			name:    "mkvextract error retries",
			message: "MKVExtract unexpected error during extraction",
			want:    true,
		},
		{
			name:    "download 500 retries",
			message: "download code 500",
			want:    true,
		},
		{
			name:    "connection refused retries",
			message: "connection refused",
			want:    true,
		},
		{
			name:    "checksum error retries",
			message: "CHecksum error on download source",
			want:    true,
		},
		{
			name:    "GLIBC not found retries",
			message: "GLIBC_2.34 not found",
			want:    true,
		},
		{
			name:    "received signal 15 retries",
			message: "received signal 15",
			want:    true,
		},
		{
			name:    "bin_data does not retry",
			message: "Data: bin_data found in stream",
			want:    false,
		},
		{
			name:    "scale/rate 0/0 does not retry",
			message: "scale/rate is 0/0 which is invalid",
			want:    false,
		},
		{
			name:    "probably corrupt does not retry",
			message: "probably corrupt input file",
			want:    false,
		},
		{
			name:    "with 0 items does not retry",
			message: "completed with 0 items",
			want:    false,
		},
		{
			name:    "exit status 1 empty stderr retries",
			message: "exit status 1: stder: stdout:",
			want:    true,
		},
		{
			name:    "nil pointer retries",
			message: "runtime error: invalid memory address or nil pointer dereference",
			want:    true,
		},
		{
			name:    "maybe incorrect parameters retries",
			message: "maybe incorrect parameters such as bit_rate, rate, width or height",
			want:    true,
		},
		{
			name:    "no such host retries",
			message: "dial tcp: lookup foo.bar: no such host",
			want:    true,
		},
		{
			name:    "i/o timeout retries",
			message: "i/o timeout connecting to server",
			want:    true,
		},
		{
			name:    "Errorf while decoding does not retry",
			message: "Errorf while decoding stream #0:1",
			want:    false,
		},
		{
			name:    "source file duration does not retry",
			message: "source File duration mismatch",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyFailureMessage(tt.message)
			if got != tt.want {
				t.Errorf("verifyFailureMessage(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestFormatTargetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "x264 to x265 mkv",
			input: "movies/test.x264.avi",
			want:  "movies/test.x265.mkv",
		},
		{
			name:  "h264 to x265 mkv",
			input: "movies/test.h264.mp4",
			want:  "movies/test.x265.mkv",
		},
		{
			name:  "ac3 to AAC",
			input: "movies/test.ac3.mkv",
			want:  "movies/test.AAC.mkv",
		},
		{
			name:  "no codec match same extension returns unchanged",
			input: "movies/test.mkv",
			want:  "movies/test.mkv",
		},
		{
			name:  "no codec match different extension changes to mkv",
			input: "movies/test.avi",
			want:  "movies/test.mkv",
		},
		{
			name:  "multiple replacements",
			input: "shows/Season 1/show.x264.dts.avi",
			want:  "shows/Season 1/show.x265.AAC.mkv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTargetName(tt.input)
			if got != tt.want {
				t.Errorf("formatTargetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
