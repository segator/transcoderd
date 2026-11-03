package worker

import (
	"context"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"
)

func TestPauseHoldsPrefetchedEncodeUntilResume(t *testing.T) {
	for _, target := range []int{1, 4} {
		t.Run(string(rune('0'+target)), func(t *testing.T) {
			executor := step.NewStepExecutor(model.FFMPEGSNotification, nil)
			worker := testAdmissionWorker(target)
			started := make(chan *job.Context, target+1)
			release := make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer close(release)
			for i := 0; i < target+1; i++ {
				executor.AddJob(&job.Context{})
			}
			go runDynamicStepQueue(ctx, executor, worker.encodePool, func(j *job.Context) {
				started <- j
				select {
				case <-release:
				case <-ctx.Done():
				}
			})
			for i := 0; i < target; i++ {
				waitForEncode(t, started)
			}
			if err := worker.SetMaxActiveJobs(0); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < target; i++ {
				release <- struct{}{}
			}
			waitForEncodeCount(t, worker.encodePool, 0)
			assertNoEncodeStarted(t, started)
			if !worker.AdmissionStatus().Paused || worker.ReserveJobSlot() {
				t.Fatal("pause did not hold admission and queued encodes")
			}
			if err := worker.SetMaxActiveJobs(target); err != nil {
				t.Fatal(err)
			}
			waitForEncode(t, started)
			if worker.AdmissionStatus().Paused {
				t.Fatal("Resume left encode queue paused")
			}
		})
	}
}

func TestMaintenanceDrainsPausedAcceptedEncodes(t *testing.T) {
	tests := []struct {
		name  string
		drain func(*JobExecutor) error
	}{
		{"configuration drain", (*JobExecutor).DrainAcceptedJobs},
		{"mandatory update hold", (*JobExecutor).HoldAdmission},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testAdmissionWorker(1)
			if err := worker.SetMaxActiveJobs(0); err != nil {
				t.Fatal(err)
			}
			executor := step.NewStepExecutor(model.FFMPEGSNotification, nil)
			executor.AddJob(&job.Context{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan *job.Context, 1)
			go runDynamicStepQueue(ctx, executor, worker.encodePool, func(j *job.Context) { started <- j })
			assertNoEncodeStarted(t, started)
			if err := tt.drain(worker); err != nil {
				t.Fatal(err)
			}
			waitForEncode(t, started)
			if worker.ReserveJobSlot() {
				t.Fatal("maintenance reopened job requests")
			}
		})
	}
}

func TestPausedStartupHoldsRestoredEncode(t *testing.T) {
	client := serverclient.NewServerClient(&web.Config{Domain: "http://127.0.0.1"}, "paused")
	worker := NewEncodeWorker(&config.Config{Name: "paused", TemporalPath: t.TempDir(), MaxActiveJobs: 0, EncodeConfig: &config.FFMPEGConfig{ParallelJobs: 4}, PGSConfig: &config.PGSConfig{}}, client, nil)
	executor := worker.stepExecutors[model.FFMPEGSNotification]
	executor.AddJob(&job.Context{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan *job.Context, 1)
	go runDynamicStepQueue(ctx, executor, worker.encodePool, func(j *job.Context) { started <- j })
	assertNoEncodeStarted(t, started)
	if !worker.AdmissionStatus().Paused {
		t.Fatal("zero startup did not retain pause")
	}
	if err := worker.SetMaxActiveJobs(4); err != nil {
		t.Fatal(err)
	}
	waitForEncode(t, started)
}

func waitForEncode(t *testing.T, started <-chan *job.Context) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected encode did not start")
	}
}

func waitForEncodeCount(t *testing.T, pool *encodeConcurrency, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		active, _ := pool.Status()
		if active == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("active encodes = %d, want %d", active, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRestoredEncodeWaitsForPersistedSettings(t *testing.T) {
	worker := testAdmissionWorker(4)
	worker.RequireWorkerSettings()
	executor := step.NewStepExecutor(model.FFMPEGSNotification, nil)
	executor.AddJob(&job.Context{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan *job.Context, 1)
	go runDynamicStepQueue(ctx, executor, worker.encodePool, func(j *job.Context) { started <- j })
	assertNoEncodeStarted(t, started)
	if err := worker.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	worker.MarkWorkerSettingsResolved()
	assertNoEncodeStarted(t, started)
	if err := worker.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	waitForEncode(t, started)
}
