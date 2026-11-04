package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"transcoder/model"
	"transcoder/server/library"
	"transcoder/server/repository"
)

func TestReviewedMediaQueueConcurrencyRetriesAndFingerprint(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "clip.mov")
	if err := os.WriteFile(path, []byte("reviewed-file"), 0600); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewInMemoryRepository()
	s, err := NewScheduler(&Config{SourcePath: root, MinFileSize: 10}, repo)
	if err != nil {
		t.Fatal(err)
	}
	file := model.MediaFile{Root: "source", Path: "clip.mov", Size: stat.Size(), ModifiedNS: stat.ModTime().UnixNano(), Metadata: &model.MediaMetadata{Probe: &model.MediaProbe{DurationSeconds: 10}}}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.QueueMediaFile(ctx, file, model.MediaFilters{}); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent queue successes = %d, want 1", successes)
	}
	j, err := repo.GetJobByPath(ctx, file.Path)
	if err != nil || j == nil {
		t.Fatalf("job = %+v, %v", j, err)
	}
	if j.SourceProbe == nil || j.SourceProbe.DurationSeconds != 10 || j.TargetPath == file.Path {
		t.Fatalf("reviewed probe/target = %+v", j)
	}
	if err := repo.AddNewTaskEvent(ctx, j.AddEvent(model.JobNotification, model.FailedNotificationStatus)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueMediaFile(ctx, file, model.MediaFilters{}); err == nil {
		t.Fatal("implicit failed retry accepted")
	}
	if _, err := s.QueueMediaFile(ctx, file, model.MediaFilters{RetryFailed: true}); err != nil {
		t.Fatal(err)
	}
	file.Root = "encoded"
	if _, err := s.QueueMediaFile(ctx, file, model.MediaFilters{}); err != library.ErrInvalidPath {
		t.Errorf("output intake = %v", err)
	}
	file.Root = "source"
	file.Size++
	if _, err := s.QueueMediaFile(ctx, file, model.MediaFilters{}); err != library.ErrChanged {
		t.Errorf("stale fingerprint = %v", err)
	}
}
