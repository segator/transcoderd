package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"

	"github.com/google/uuid"
)

func TestProtectedDirectorySyncRejectsMissingAndNonDirectories(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := syncProtectedDirectory(root, "."); err != nil {
		t.Fatalf("syncProtectedDirectory(root) = %v", err)
	}
	if err := syncProtectedDirectory(root, "file"); err == nil {
		t.Fatal("syncProtectedDirectory() accepted a regular file")
	}
	if err := syncProtectedDirectory(root, "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("syncProtectedDirectory(missing) = %v", err)
	}
}

func TestProtectedCleanupPreservesSourceWhenFinalOutputIsMissingOrCorrupt(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(".transcoderd-upload", "movie.g1")
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(stage)), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"source.mkv": []byte("source"), stage: []byte("encoded")} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256([]byte("encoded"))
	publication := model.ProtectedPublication{JobID: uuid.New(), Generation: 1, StagingPath: stage, FinalPath: "final.mkv", SizeBytes: 7, Checksum: hex.EncodeToString(sum[:]), DeleteSource: true, CleanupPending: true}
	job := &model.Job{Id: publication.JobID, SourcePath: "source.mkv"}
	scheduler, err := NewScheduler(&Config{SourcePath: root}, repository.NewInMemoryRepository())
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []bool{true, false} {
		if !missing {
			if err := os.WriteFile(filepath.Join(root, "final.mkv"), []byte("damaged"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := scheduler.finishProtectedCleanup(context.Background(), publication, job); err == nil {
			t.Fatalf("cleanup succeeded with missing=%t final output", missing)
		}
		for _, path := range []string{"source.mkv", stage} {
			if _, err := os.Stat(filepath.Join(root, path)); err != nil {
				t.Fatalf("cleanup removed %s with missing=%t: %v", path, missing, err)
			}
		}
	}
}
