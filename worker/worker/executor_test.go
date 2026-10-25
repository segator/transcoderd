package worker

import (
	"context"
	"os"
	"path/filepath"
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

func TestInitialEncodeConcurrencyUsesConfiguredLimit(t *testing.T) {
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
			pool := newEncodeConcurrency(e.workerConfig.EncodeParallelJobs())
			if _, got := pool.Status(); got != tt.want {
				t.Errorf("initial encode limit = %d, want %d", got, tt.want)
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

func TestCleanJobReleasesActiveSlotAfterCleanup(t *testing.T) {
	executor := &JobExecutor{}
	atomic.StoreUint32(&executor.activeJobs, 1)

	cleaned := false
	err := executor.cleanJob(func() error {
		if got := executor.ActiveJobs(); got != 1 {
			t.Errorf("ActiveJobs() during cleanup = %d, want 1", got)
		}
		cleaned = true
		return nil
	})
	if err != nil {
		t.Fatalf("cleanJob() error = %v", err)
	}
	if !cleaned {
		t.Fatal("cleanup was not called")
	}
	if got := executor.ActiveJobs(); got != 0 {
		t.Errorf("ActiveJobs() after cleanup = %d, want 0", got)
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

func TestRuntimeEncodeLimitScalesWithoutInterruptingActiveJobs(t *testing.T) {
	executor := step.NewStepExecutor(model.FFMPEGSNotification, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newEncodeConcurrency(2)
	worker := &JobExecutor{workerConfig: &config.Config{}, maxActiveJobs: 2, encodePool: pool}
	started := make(chan *job.Context, 4)
	finished := make(chan *job.Context, 4)
	release := make(map[*job.Context]chan struct{})
	jobs := []*job.Context{{}, {}, {}, {}}
	for _, jobContext := range jobs {
		release[jobContext] = make(chan struct{})
		executor.AddJob(jobContext)
	}
	process := func(j *job.Context) {
		started <- j
		<-release[j]
		finished <- j
	}
	go runDynamicStepQueue(ctx, executor, pool, process)
	startedJobs := make([]*job.Context, 0, 3)
	for i := 0; i < 2; i++ {
		select {
		case jobContext := <-started:
			startedJobs = append(startedJobs, jobContext)
		case <-time.After(2 * time.Second):
			t.Fatal("two encoding jobs did not overlap")
		}
	}
	select {
	case <-started:
		t.Fatal("third job started before an encode slot was released")
	case <-time.After(30 * time.Millisecond):
	}
	if err := worker.SetMaxActiveJobs(3); err != nil {
		t.Fatal(err)
	}
	select {
	case jobContext := <-started:
		startedJobs = append(startedJobs, jobContext)
	case <-time.After(2 * time.Second):
		t.Fatal("raising the runtime limit did not start the third encoder")
	}
	if status := worker.AdmissionStatus(); status.MaxEncodes != 3 || status.ActiveEncodes != 3 {
		t.Errorf("status after raising limit = encodes %d/%d, want 3/3", status.ActiveEncodes, status.MaxEncodes)
	}
	if err := worker.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	close(release[startedJobs[0]])
	<-finished
	assertNoEncodeStarted(t, started)
	close(release[startedJobs[1]])
	<-finished
	assertNoEncodeStarted(t, started)
	close(release[startedJobs[2]])
	<-finished
	select {
	case jobContext := <-started:
		if jobContext != jobs[3] {
			t.Errorf("queued job = %p, want %p", jobContext, jobs[3])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued encoder did not start after active encodes drained below target")
	}
	if status := worker.AdmissionStatus(); status.MaxEncodes != 1 || status.ActiveEncodes != 1 {
		t.Errorf("status after lowering limit = encodes %d/%d, want 1/1", status.ActiveEncodes, status.MaxEncodes)
	}
	close(release[jobs[3]])
	<-finished
	if len(startedJobs) != 3 {
		t.Errorf("started %d encoding jobs before lowering, want 3", len(startedJobs))
	}
	cancel()
}

func assertNoEncodeStarted(t *testing.T, started <-chan *job.Context) {
	t.Helper()
	select {
	case jobContext := <-started:
		t.Fatalf("job %p started while active encodes were at the lowered limit", jobContext)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestNonEncodeStepQueueProcessesOneJobAtATime(t *testing.T) {
	executor := step.NewStepExecutor(model.DownloadNotification, nil)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan *job.Context, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	first, second := &job.Context{}, &job.Context{}
	executor.AddJob(first)
	executor.AddJob(second)
	go func() {
		runStepQueue(ctx, executor, func(current *job.Context) {
			started <- current
			<-release
		})
		close(done)
	}()
	select {
	case got := <-started:
		if got != first {
			t.Fatalf("first processed job = %p, want %p", got, first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first non-FFmpeg job did not start")
	}
	select {
	case got := <-started:
		t.Fatalf("second job %p started before the first finished", got)
	case <-time.After(30 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case got := <-started:
		if got != second {
			t.Fatalf("second processed job = %p, want %p", got, second)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second non-FFmpeg job did not start after the first finished")
	}
	release <- struct{}{}
	cancel()
	<-done
}
