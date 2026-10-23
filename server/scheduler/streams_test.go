package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestUploadStream(t *testing.T, dir, finalPath, name string) *UploadJobStream {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	file, err := root.Create(name)
	if err != nil {
		_ = root.Close()
		t.Fatal(err)
	}
	return &UploadJobStream{
		JobStream:   &JobStream{file: file},
		root:        root,
		attemptName: name,
		stagingName: filepath.Base(finalPath),
	}
}

func TestUploadAttemptsHaveIsolatedStagingFiles(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "job.upload")

	failedAttempt := newTestUploadStream(t, dir, finalPath, "attempt-failed.tmp")
	if _, err := failedAttempt.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	if err := failedAttempt.Close(false); err != nil {
		t.Fatal(err)
	}

	successfulAttempt := newTestUploadStream(t, dir, finalPath, "attempt-success.tmp")
	if _, err := successfulAttempt.Write([]byte("complete upload")); err != nil {
		t.Fatal(err)
	}
	if err := successfulAttempt.Close(false); err != nil {
		t.Fatal(err)
	}
	if err := successfulAttempt.Commit(); err != nil {
		t.Fatal(err)
	}

	if err := failedAttempt.Clean(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("successful attempt was removed by failed attempt cleanup: %v", err)
	}
	if string(contents) != "complete upload" {
		t.Errorf("committed contents = %q, want complete upload", contents)
	}
	if err := successfulAttempt.Clean(); err != nil {
		t.Fatal(err)
	}
	if contents, err = os.ReadFile(finalPath); err != nil || string(contents) != "complete upload" {
		t.Errorf("cleaning the attempt file changed the committed staging file: contents=%q err=%v", contents, err)
	}
}

func TestUploadCommitIsIdempotentAndRejectsDifferentContents(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "job.upload")

	first := newTestUploadStream(t, dir, finalPath, "attempt-first.tmp")
	if _, err := first.Write([]byte("same upload")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(false); err != nil {
		t.Fatal(err)
	}
	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := first.Clean(); err != nil {
		t.Fatal(err)
	}

	duplicate := newTestUploadStream(t, dir, finalPath, "attempt-duplicate.tmp")
	if _, err := duplicate.Write([]byte("same upload")); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.Close(false); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.Commit(); err != nil {
		t.Fatalf("committing identical retry should be idempotent: %v", err)
	}
	if err := duplicate.Clean(); err != nil {
		t.Fatal(err)
	}

	different := newTestUploadStream(t, dir, finalPath, "attempt-different.tmp")
	if _, err := different.Write([]byte("different upload")); err != nil {
		t.Fatal(err)
	}
	if err := different.Close(false); err != nil {
		t.Fatal(err)
	}
	if err := different.Commit(); err == nil {
		t.Fatal("committing different contents over an existing staged upload succeeded")
	}
	if err := different.Clean(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(finalPath)
	if err != nil || string(contents) != "same upload" {
		t.Errorf("existing staged contents changed: contents=%q err=%v", contents, err)
	}
}

func TestUploadCommitCannotEscapeRoot(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "root")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	upload := newTestUploadStream(t, dir, filepath.Join(dir, "job.upload"), "attempt.tmp")
	upload.stagingName = "../escaped.upload"
	if _, err := upload.Write([]byte("upload")); err != nil {
		t.Fatal(err)
	}
	if err := upload.Close(false); err != nil {
		t.Fatal(err)
	}
	if err := upload.Commit(); err == nil {
		t.Fatal("Commit() escaped the upload root")
	}
	if err := upload.Clean(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escaped.upload")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaped upload exists or could not be checked: %v", err)
	}
}
