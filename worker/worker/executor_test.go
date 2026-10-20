package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/step"
)

func TestAcceptJobsCombinesLimitAndWindow(t *testing.T) {
	start, stop := time.Duration(0), 24*time.Hour
	outsideStart, outsideStop := 24*time.Hour, 24*time.Hour
	tests := []struct {
		name   string
		paused bool
		active uint32
		limit  int
		start  *time.Duration
		stop   *time.Duration
		want   bool
	}{
		{"available", false, 1, 2, nil, nil, true},
		{"paused", true, 0, 2, nil, nil, false},
		{"at limit", false, 2, 2, nil, nil, false},
		{"custom limit", false, 3, 4, nil, nil, true},
		{"inside window at limit", false, 2, 2, &start, &stop, false},
		{"inside window available", false, 1, 2, &start, &stop, true},
		{"outside window", false, 0, 2, &outsideStart, &outsideStop, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &JobExecutor{workerConfig: &config.Config{Paused: tt.paused, MaxActiveJobs: tt.limit, StartAfter: tt.start, StopAfter: tt.stop}, activeJobs: tt.active}
			if got := e.AcceptJobs(); got != tt.want {
				t.Errorf("AcceptJobs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeConsumerLimit(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{EncodeConfig: &config.FFMPEGConfig{ParallelJobs: 2}}}
	if got := e.stepConsumers(model.FFMPEGSNotification); got != 2 {
		t.Errorf("stepConsumers(FFMPEG) = %d, want 2", got)
	}
	if got := e.stepConsumers(model.UploadNotification); got != 1 {
		t.Errorf("stepConsumers(Upload) = %d, want 1", got)
	}
}

func TestAcceptJobsCombinesWeeklyScheduleWithPauseAndLimit(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{
		MaxActiveJobs: 4,
		Schedule:      &config.ScheduleConfig{TimeZone: "Europe/Madrid", Windows: []string{"Mon-Fri@00:00-10:00"}},
	}, activeJobs: 3}
	before := time.Date(2026, 10, 19, 7, 59, 59, 0, time.UTC)
	if !e.acceptJobsAt(before) || e.acceptJobsAt(before.Add(time.Second)) {
		t.Error("acceptJobsAt() did not close admission at 10:00 local time")
	}
	if e.ActiveJobs() != 3 {
		t.Error("closing admission changed accepted job count")
	}
	e.workerConfig.Paused = true
	if e.acceptJobsAt(before) {
		t.Error("acceptJobsAt() permitted a paused worker inside a weekly window")
	}
	e.workerConfig.Paused = false
	atomic.StoreUint32(&e.activeJobs, 4)
	if e.acceptJobsAt(before) {
		t.Error("acceptJobsAt() bypassed the active-job limit inside a weekly window")
	}
}

func TestParallelStepQueuesOverlapAndKeepJobState(t *testing.T) {
	executor := step.NewStepExecutor(model.FFMPEGSNotification, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan *job.Context, 3)
	release := make(chan struct{})
	finished := make(chan *job.Context, 3)
	var running, peak int32
	process := func(j *job.Context) {
		current := atomic.AddInt32(&running, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if current <= old || atomic.CompareAndSwapInt32(&peak, old, current) {
				break
			}
		}
		started <- j
		<-release
		atomic.AddInt32(&running, -1)
		finished <- j
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); runStepQueue(ctx, executor, process) }()
	}
	jobs := []*job.Context{{}, {}, {}}
	for _, j := range jobs {
		executor.AddJob(j)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("two encoding jobs did not overlap")
		}
	}
	select {
	case <-started:
		t.Fatal("third job started before an encode slot was released")
	case <-time.After(30 * time.Millisecond):
	}
	if got := atomic.LoadInt32(&peak); got != 2 {
		t.Errorf("peak simultaneous jobs = %d, want 2", got)
	}
	close(release)
	seen := make(map[*job.Context]bool)
	for i := 0; i < 3; i++ {
		select {
		case j := <-finished:
			seen[j] = true
		case <-time.After(2 * time.Second):
			t.Fatal("queued job did not finish")
		}
	}
	for _, j := range jobs {
		if !seen[j] {
			t.Error("job result lost between encoding consumers")
		}
	}
	cancel()
	wg.Wait()
}
