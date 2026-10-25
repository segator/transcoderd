package repository

import (
	"context"
	"testing"
)

func TestInMemoryRepositoryWorkerMaxActiveJobs(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	if got, err := repo.GetWorkerMaxActiveJobs(ctx, "gpu"); err != nil || got != nil {
		t.Fatalf("GetWorkerMaxActiveJobs() = %v, %v; want no setting", got, err)
	}
	if err := repo.SetWorkerMaxActiveJobs(ctx, "gpu", 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWorkerMaxActiveJobs(ctx, "cpu", 3); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]int{"gpu": 0, "cpu": 3} {
		got, err := repo.GetWorkerMaxActiveJobs(ctx, name)
		if err != nil {
			t.Fatalf("GetWorkerMaxActiveJobs(%q): %v", name, err)
		}
		if got == nil || *got != want {
			t.Errorf("GetWorkerMaxActiveJobs(%q) = %v, want %d", name, got, want)
		}
	}
}

func TestInMemoryRepositoryRejectsInvalidWorkerMaxActiveJobs(t *testing.T) {
	repo := NewInMemoryRepository()
	if err := repo.SetWorkerMaxActiveJobs(context.Background(), "gpu", -1); err == nil {
		t.Fatal("SetWorkerMaxActiveJobs() accepted a negative maximum")
	}
	if err := repo.SetWorkerMaxActiveJobs(context.Background(), "", 1); err == nil {
		t.Fatal("SetWorkerMaxActiveJobs() accepted an empty worker name")
	}
}
