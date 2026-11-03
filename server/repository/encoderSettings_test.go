package repository

import (
	"testing"

	"transcoder/model"
)

func TestIndependentWorkerSettingsIsolationAndValidation(t *testing.T) {
	r := NewInMemoryRepository()
	ctx := t.Context()
	cpu, gpu := 2, 3
	if err := r.SetWorkerSettings(ctx, "mixed", model.WorkerSettings{CPUJobs: &cpu, GPUJobs: &gpu}); err != nil {
		t.Fatal(err)
	}
	cpu, gpu = 99, 99
	got, err := r.GetWorkerSettings(ctx, "mixed")
	if err != nil || *got.CPUJobs != 2 || *got.GPUJobs != 3 || *got.MaxActiveJobs != 5 {
		t.Fatalf("stored pair = %+v, %v", got, err)
	}
	*got.CPUJobs = 100
	again, _ := r.GetWorkerSettings(ctx, "mixed")
	if *again.CPUJobs != 2 {
		t.Error("settings read exposed mutable persisted pointers")
	}
	notSet, _ := r.GetWorkerSettings(ctx, "other")
	if notSet.MaxActiveJobs != nil || notSet.Independent() {
		t.Errorf("other settings = %+v", notSet)
	}
	negative, max, one := -1, int(^uint(0)>>1), 1
	for _, settings := range []model.WorkerSettings{{}, {CPUJobs: &one}, {CPUJobs: &negative, GPUJobs: &one}, {CPUJobs: &max, GPUJobs: &one}, {CPUJobs: &one, GPUJobs: &one, MaxActiveJobs: &one}} {
		if err := r.SetWorkerSettings(ctx, "mixed", settings); err == nil {
			t.Errorf("SetWorkerSettings() accepted %+v", settings)
		}
	}
	if err := r.SetWorkerMaxActiveJobs(ctx, "mixed", 4); err != nil {
		t.Fatal(err)
	}
	again, _ = r.GetWorkerSettings(ctx, "mixed")
	if again.Independent() || again.CPUJobs != nil || again.GPUJobs != nil || *again.MaxActiveJobs != 4 {
		t.Errorf("legacy setting did not clear pair: %+v", again)
	}
}
