package worker

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/step"
)

func TestReserveJobSlotCombinesLimitAndWindow(t *testing.T) {
	start, stop := time.Duration(0), 24*time.Hour
	outsideStart, outsideStop := 24*time.Hour, 24*time.Hour
	tests := []struct {
		name   string
		active uint32
		limit  int
		start  *time.Duration
		stop   *time.Duration
		want   bool
	}{
		{"available", 1, 2, nil, nil, true},
		{"zero maximum", 0, 0, nil, nil, false},
		{"at limit", 2, 2, nil, nil, false},
		{"custom limit", 3, 4, nil, nil, true},
		{"inside window at limit", 2, 2, &start, &stop, false},
		{"inside window available", 1, 2, &start, &stop, true},
		{"outside window", 0, 2, &outsideStart, &outsideStop, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: tt.limit, StartAfter: tt.start, StopAfter: tt.stop}, activeJobs: tt.active, maxActiveJobs: tt.limit}
			if got := e.ReserveJobSlot(); got != tt.want {
				t.Errorf("ReserveJobSlot() = %v, want %v", got, tt.want)
			}
			if tt.want {
				e.ReleaseJobSlot()
			}
		})
	}
}

func TestEncodeConsumerLimit(t *testing.T) {
	tests := []struct {
		name                   string
		active, override, want int
	}{
		{"GPU defaults follow four active jobs", 4, 0, 4},
		{"CPU defaults follow two active jobs", 2, 0, 2},
		{"serial defaults", 1, 0, 1},
		{"zero admission still resumes persisted jobs", 0, 0, 1},
		{"explicit lower limit", 4, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &JobExecutor{workerConfig: &config.Config{MaxActiveJobs: tt.active, EncodeConfig: &config.FFMPEGConfig{ParallelJobs: tt.override}}}
			if got := e.stepConsumers(model.FFMPEGSNotification); got != tt.want {
				t.Errorf("stepConsumers(FFMPEG) = %d, want %d", got, tt.want)
			}
			if got := e.stepConsumers(model.UploadNotification); got != 1 {
				t.Errorf("stepConsumers(Upload) = %d, want 1", got)
			}
		})
	}
}

func TestResumeJobsDiscardsTerminalJobContext(t *testing.T) {
	executor, downloadExecutor, contextPath := newResumeTestExecutor(t)
	jobContext := persistResumeTestContext(t, executor.tempPath, model.JobNotification, model.CompletedNotificationStatus)

	executor.resumeJobs()

	if got := executor.ActiveJobs(); got != 0 {
		t.Errorf("ActiveJobs() = %d, want 0", got)
	}
	if got := len(downloadExecutor.GetJobChan()); got != 0 {
		t.Errorf("queued download jobs = %d, want 0", got)
	}
	if _, err := os.Stat(contextPath(jobContext)); !os.IsNotExist(err) {
		t.Errorf("terminal job context still exists, stat error = %v", err)
	}
}

func TestResumeJobsRoutesStartedJobContextToDownload(t *testing.T) {
	executor, downloadExecutor, _ := newResumeTestExecutor(t)
	persistResumeTestContext(t, executor.tempPath, model.JobNotification, model.StartedNotificationStatus)

	executor.resumeJobs()

	if got := executor.ActiveJobs(); got != 1 {
		t.Errorf("ActiveJobs() = %d, want 1", got)
	}
	if got := len(downloadExecutor.GetJobChan()); got != 1 {
		t.Errorf("queued download jobs = %d, want 1", got)
	}
}

func TestResumeJobsSkipsUnreadableAndUnsupportedContexts(t *testing.T) {
	executor, downloadExecutor, _ := newResumeTestExecutor(t)
	if err := os.WriteFile(filepath.Join(executor.tempPath, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	persistResumeTestContext(t, executor.tempPath, model.NotificationType("Unknown"), model.StartedNotificationStatus)

	executor.resumeJobs()

	if got := executor.ActiveJobs(); got != 0 {
		t.Errorf("ActiveJobs() = %d, want 0", got)
	}
	if got := len(downloadExecutor.GetJobChan()); got != 0 {
		t.Errorf("queued download jobs = %d, want 0", got)
	}
}

func newResumeTestExecutor(t *testing.T) (*JobExecutor, *step.Executor, func(*job.Context) string) {
	t.Helper()
	tempPath := filepath.Join(t.TempDir(), "worker-test")
	if err := os.MkdirAll(tempPath, 0o700); err != nil {
		t.Fatal(err)
	}
	downloadExecutor := step.NewStepExecutor(model.DownloadNotification, nil)
	executor := &JobExecutor{
		tempPath: tempPath,
		stepExecutors: map[model.NotificationType]*step.Executor{
			model.DownloadNotification: downloadExecutor,
		},
	}
	return executor, downloadExecutor, func(jobContext *job.Context) string {
		return filepath.Join(jobContext.WorkingDir, jobContext.JobId.String()+".json")
	}
}

func persistResumeTestContext(t *testing.T, tempPath string, notificationType model.NotificationType, status model.NotificationStatus) *job.Context {
	t.Helper()
	jobID := uuid.New()
	jobContext := job.NewContext(jobID, 0, filepath.Join(tempPath, jobID.String()))
	if err := jobContext.Init(); err != nil {
		t.Fatal(err)
	}
	jobContext.UpdateEvent(notificationType, status, "")
	if err := jobContext.PersistJobContext(); err != nil {
		t.Fatal(err)
	}
	return jobContext
}

func TestAdmissionCombinesWeeklyScheduleWithLimit(t *testing.T) {
	e := &JobExecutor{workerConfig: &config.Config{
		MaxActiveJobs: 4,
		Schedule:      &config.ScheduleConfig{TimeZone: "Europe/Madrid", Windows: []string{"Mon-Fri@00:00-10:00"}},
	}, activeJobs: 3, maxActiveJobs: 4}
	before := time.Date(2026, 10, 19, 7, 59, 59, 0, time.UTC)
	if !e.canReserveJobSlotAt(before) {
		t.Fatal("worker did not reserve a slot inside its weekly window")
	}
	if e.canReserveJobSlotAt(before.Add(time.Second)) {
		t.Error("admission did not close at 10:00 local time")
	}
	if e.ActiveJobs() != 3 {
		t.Error("closing admission changed accepted job count")
	}
	atomic.StoreUint32(&e.activeJobs, 4)
	if e.canReserveJobSlotAt(before) {
		t.Error("admission bypassed the active-job limit inside a weekly window")
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
