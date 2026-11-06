package repository

import (
	"testing"

	"transcoder/model"

	"github.com/google/uuid"
)

func checkWorkerControlRepository(t *testing.T, repo Repository) {
	t.Helper()
	ctx := t.Context()
	cpu, gpu := 2, 3
	if err := repo.SetWorkerSettings(ctx, "remote", model.WorkerSettings{CPUJobs: &cpu, GPUJobs: &gpu}); err != nil {
		t.Fatal(err)
	}
	zero := 0
	if err := repo.SetWorkerSettings(ctx, "remote", model.WorkerSettings{CPUJobs: &zero, GPUJobs: &zero}); err != nil {
		t.Fatal(err)
	}
	restore, err := repo.GetWorkerResumeSettings(ctx, "remote")
	if err != nil || !restore.Independent() || *restore.CPUJobs != 2 || *restore.GPUJobs != 3 {
		t.Fatalf("GetWorkerResumeSettings() = %+v, %v", restore, err)
	}
	c := model.WorkerControl{ID: uuid.NewString(), Action: "pause-encodes"}
	if err := repo.SetWorkerControl(ctx, "remote", c); err != nil {
		t.Fatal(err)
	}
	oldID := c.ID
	c.ID = uuid.NewString()
	c.Action = "resume-encodes"
	if err := repo.SetWorkerControl(ctx, "remote", c); err != nil {
		t.Fatal(err)
	}
	if err := repo.AckWorkerControl(ctx, "remote", model.WorkerControlAck{ID: oldID}); err == nil {
		t.Fatal("stale acknowledgement accepted")
	}
	got, err := repo.GetWorkerControl(ctx, "remote")
	if err != nil || got.ID != c.ID || got.State != "pending" {
		t.Fatalf("GetWorkerControl() = %+v, %v", got, err)
	}
	if err := repo.AckWorkerControl(ctx, "remote", model.WorkerControlAck{ID: c.ID, Error: "OS refused suspension"}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetWorkerControl(ctx, "remote")
	if err != nil || got.State != "failed" || got.Error != "OS refused suspension" {
		t.Fatalf("failed control = %+v, %v", got, err)
	}
	if err := repo.AckWorkerControl(ctx, "remote", model.WorkerControlAck{ID: c.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetWorkerControl(ctx, "remote")
	if got.State != "applied" {
		t.Fatalf("control state = %s", got.State)
	}
	c.ID, c.Action, c.JobID = uuid.NewString(), "pause-job", uuid.NewString()
	if err := repo.SetWorkerControl(ctx, "remote", c); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetWorkerControl(ctx, "remote")
	if err != nil || got.JobID != c.JobID || got.Action != "pause-job" {
		t.Fatalf("individual control=%+v %v", got, err)
	}

	if err := repo.SetWorkerMaxActiveJobs(ctx, "remote", 4); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWorkerMaxActiveJobs(ctx, "remote", 0); err != nil {
		t.Fatal(err)
	}
	restore, err = repo.GetWorkerResumeSettings(ctx, "remote")
	if err != nil || restore.Independent() || restore.MaxActiveJobs == nil || *restore.MaxActiveJobs != 4 {
		t.Fatalf("legacy restore = %+v %v", restore, err)
	}
}
func TestWorkerControlRepository(t *testing.T) {
	checkWorkerControlRepository(t, NewInMemoryRepository())
}
