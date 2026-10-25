package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/control"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type JobExecutor struct {
	activeJobs    uint32
	reservedJobs  int
	maxActiveJobs int

	workerConfig *config.Config
	tempPath     string
	wg           sync.WaitGroup
	mu           sync.RWMutex
	admissionMu  sync.Mutex
	client       *serverclient.ServerClient

	console       *console.RenderService
	stepExecutors map[model.NotificationType]*step.Executor
}

func NewEncodeWorker(workerConfig *config.Config, client *serverclient.ServerClient, renderService *console.RenderService) *JobExecutor {
	tempPath := filepath.Join(workerConfig.TemporalPath, fmt.Sprintf("worker-%s", workerConfig.Name))

	jobExecutor := &JobExecutor{
		client:        client,
		wg:            sync.WaitGroup{},
		workerConfig:  workerConfig,
		stepExecutors: make(map[model.NotificationType]*step.Executor),
		tempPath:      tempPath,
		console:       renderService,
		activeJobs:    0,
		maxActiveJobs: workerConfig.MaxActiveJobs,
	}
	stepExecutors := setupStepExecutors(jobExecutor)
	jobExecutor.stepExecutors = stepExecutors

	if err := os.MkdirAll(tempPath, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	return jobExecutor
}

func setupStepExecutors(jobExecutor *JobExecutor) map[model.NotificationType]*step.Executor {
	workerConfig := jobExecutor.workerConfig
	client := jobExecutor.client

	onErrOpt := step.WithOnErrorOpt(func(jobContext *job.Context, notificationType model.NotificationType, err error) {
		jobExecutor.publishTaskEvent(jobContext, model.JobNotification, model.FailedNotificationStatus, fmt.Sprintf("%s:%s", notificationType, err.Error()))
		jobExecutor.ConsoleTrackStep(jobContext.JobId.String(), model.JobNotification).Error()
		if err := jobExecutor.CleanJob(jobContext); err != nil {
			jobExecutor.jobLogger(jobContext).Errorf("failed to clean job workspace %v", err)
		}
	})
	stepExecutors := make(map[model.NotificationType]*step.Executor)

	// Download Step
	stepExecutors[model.DownloadNotification] = step.NewDownloadStepExecutor(
		workerConfig.Name,
		client.GetBaseDomain(),
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			if jobContext.Source.FFProbeData.HaveExtractableSubtitle() {
				stepExecutors[model.MKVExtractNotification].AddJob(jobContext)
				return
			}
			stepExecutors[model.FFMPEGSNotification].AddJob(jobContext)
		}),
		onErrOpt)

	// MKVExtract Step
	stepExecutors[model.MKVExtractNotification] = step.NewMKVExtractStepExecutor(onErrOpt,
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			if jobContext.Source.FFProbeData.HaveImageTypeSubtitle() {
				stepExecutors[model.PGSNotification].AddJob(jobContext)
				return
			}
			// No PGS subs (only extracted unsupported codec subs converted to SRT),
			// skip OCR and go straight to FFmpeg encoding.
			stepExecutors[model.FFMPEGSNotification].AddJob(jobContext)
		}),
		onErrOpt)

	// PGS Step
	stepExecutors[model.PGSNotification] = step.NewPGSToSrtStepExecutor(workerConfig.PGSConfig,
		step.WithParallelRunners(workerConfig.PGSConfig.ParallelJobs),
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			stepExecutors[model.FFMPEGSNotification].AddJob(jobContext)
		}),
		onErrOpt)

	// FFMPEG Step
	stepExecutors[model.FFMPEGSNotification] = step.NewFFMPEGStepExecutor(workerConfig.EncodeConfig,
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			stepExecutors[model.JobVerify].AddJob(jobContext)
		}),
		onErrOpt)

	// Verify Step
	stepExecutors[model.JobVerify] = step.NewFFMPEGVerifyStepExecutor(workerConfig.VerifyDeltaTime,
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			stepExecutors[model.UploadNotification].AddJob(jobContext)
		}),
		onErrOpt)

	// Upload Step
	stepExecutors[model.UploadNotification] = step.NewUploadStepExecutor(workerConfig.Name,
		client.GetBaseDomain(),
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			jobExecutor.publishTaskEvent(jobContext, model.JobNotification, model.CompletedNotificationStatus, "")
			jobExecutor.ConsoleTrackStep(jobContext.JobId.String(), model.JobNotification).Done()
			if err := jobExecutor.CleanJob(jobContext); err != nil {
				jobExecutor.jobLogger(jobContext).Errorf("failed to clean job workspace: %v", err)
			}
		}),
		onErrOpt)

	return stepExecutors
}

func (e *JobExecutor) Run(wg *sync.WaitGroup, ctx context.Context) {
	serviceCtx, cancelServiceCtx := context.WithCancel(context.Background())
	log.Info("Starting Worker Client...")
	e.start(serviceCtx)
	log.Info("Started Worker Client...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		cancelServiceCtx()
		e.stop()
		log.Info("Stopping Worker Client...")
		wg.Done()
	}()
}

func (e *JobExecutor) start(ctx context.Context) {
	e.resumeJobs()
	for notificationType, stepExecutor := range e.stepExecutors {
		consumers := e.stepConsumers(notificationType)
		for i := 0; i < consumers; i++ {
			go e.stepQueueRoutine(ctx, stepExecutor)
		}
	}
}

func (e *JobExecutor) stepConsumers(notificationType model.NotificationType) int {
	if notificationType == model.FFMPEGSNotification {
		return e.workerConfig.EncodeParallelJobs()
	}
	return 1
}

func (e *JobExecutor) stop() {
	for _, stepExecutor := range e.stepExecutors {
		stepExecutor.Stop()
	}
}

func (e *JobExecutor) resumeJobs() {
	err := filepath.Walk(e.tempPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".json" {
			jobContext, err := job.ReadContextFromDiskByPath(path)
			if err != nil {
				log.WithError(err).Warn("Ignoring unreadable persisted job context")
				return nil
			}
			if jobContext.LastEvent == nil {
				log.WithField("context_path", path).Warn("Ignoring persisted job context without a last event")
				return nil
			}

			notificationType := jobContext.LastEvent.NotificationType
			if notificationType == model.JobNotification {
				switch jobContext.LastEvent.Status {
				case model.AssignedNotificationStatus, model.StartedNotificationStatus:
					// A worker can stop after persisting Job/started but before it
					// persists the initial Download phase.
					notificationType = model.DownloadNotification
				case model.CompletedNotificationStatus, model.FailedNotificationStatus, model.CanceledNotificationStatus:
					log.WithFields(log.Fields{
						"context_path": path,
						"job_id":       jobContext.JobId.String(),
						"status":       jobContext.LastEvent.Status,
					}).Info("Discarding terminal persisted job context")
					if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
						log.WithError(err).WithField("context_path", path).Warn("Failed to remove terminal job context")
					}
					return nil
				default:
					log.WithFields(log.Fields{
						"context_path": path,
						"job_id":       jobContext.JobId.String(),
						"status":       jobContext.LastEvent.Status,
					}).Warn("Ignoring persisted job context with unsupported job status")
					return nil
				}
			}

			stepExecutor, ok := e.stepExecutors[notificationType]
			if !ok || stepExecutor == nil {
				log.WithFields(log.Fields{
					"context_path": path,
					"job_id":       jobContext.JobId.String(),
					"phase":        notificationType,
				}).Warn("Ignoring persisted job context with unsupported phase")
				return nil
			}

			atomic.AddUint32(&e.activeJobs, 1)
			stepExecutor.AddJob(jobContext)
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}

type AdmissionStatus = control.AdmissionStatus

func (e *JobExecutor) AdmissionStatus() AdmissionStatus {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	withinSchedule := e.workerConfig.AdmissionAllowedAt(time.Now())
	return AdmissionStatus{
		Accepting:      e.maxActiveJobs > 0 && withinSchedule,
		ActiveJobs:     e.ActiveJobs(),
		ReservedJobs:   e.reservedJobs,
		MaxActiveJobs:  e.maxActiveJobs,
		WithinSchedule: withinSchedule,
	}
}

func (e *JobExecutor) SetMaxActiveJobs(max int) error {
	if max < 0 {
		return fmt.Errorf("maximum active jobs must be non-negative")
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.maxActiveJobs = max
	return nil
}

func (e *JobExecutor) ReserveJobSlot() bool {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if !e.canReserveJobSlotAt(time.Now()) {
		return false
	}
	e.reservedJobs++
	return true
}

func (e *JobExecutor) canReserveJobSlotAt(now time.Time) bool {
	return e.maxActiveJobs > 0 && e.workerConfig.AdmissionAllowedAt(now) && int(e.ActiveJobs())+e.reservedJobs < e.maxActiveJobs
}

func (e *JobExecutor) WithinAdmissionSchedule() bool {
	return e.workerConfig.AdmissionAllowedAt(time.Now())
}

func (e *JobExecutor) ReleaseJobSlot() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.reservedJobs > 0 {
		e.reservedJobs--
	}
}
func (e *JobExecutor) jobLogger(jobContext *job.Context) console.LeveledLogger {
	return e.console.Logger(console.WithMessagePrefix(fmt.Sprintf("[%s]", jobContext.JobId.String())))
}
func (e *JobExecutor) ExecuteJob(jobId uuid.UUID, lastEvent int) error {
	if !e.ReserveJobSlot() {
		return errors.New("worker is not accepting another job")
	}
	return e.ExecuteReservedJob(jobId, lastEvent)
}

func (e *JobExecutor) ExecuteReservedJob(jobId uuid.UUID, lastEvent int) error {
	jobContext := job.NewContext(jobId, lastEvent, filepath.Join(e.tempPath, jobId.String()))

	if err := jobContext.Init(); err != nil {
		e.ReleaseJobSlot()
		return err
	}

	e.publishTaskEvent(jobContext, model.JobNotification, model.StartedNotificationStatus, "")

	e.admissionMu.Lock()
	if e.reservedJobs > 0 {
		e.reservedJobs--
	}
	atomic.AddUint32(&e.activeJobs, 1)
	e.admissionMu.Unlock()
	e.stepExecutors[model.DownloadNotification].AddJob(jobContext)
	return nil
}

func (e *JobExecutor) publishTaskEvent(jobContext *job.Context, notificationType model.NotificationType, status model.NotificationStatus, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := e.jobLogger(jobContext)
	jobContext.UpdateEvent(notificationType, status, message)
	event := &model.TaskEventType{
		Event: model.Event{
			EventTime:  time.Now(),
			WorkerName: e.workerConfig.Name,
		},
		JobId:            jobContext.JobId,
		EventID:          jobContext.LastEvent.EventId,
		NotificationType: jobContext.LastEvent.NotificationType,
		Status:           jobContext.LastEvent.Status,
		Message:          jobContext.LastEvent.Message,
	}
	if status == model.CompletedNotificationStatus && notificationType == model.JobNotification && jobContext.TargetMediaProbe != nil {
		event.TargetProbe = jobContext.TargetMediaProbe
	}
	if err := e.client.PublishTaskEvent(event); err != nil {
		l.Errorf("Error on publishing event %s", err.Error())
	}
	if err := jobContext.PersistJobContext(); err != nil {
		l.Errorf("Error on publishing event %s", err.Error())
	}
	l.Logf("%s have been %s", event.NotificationType, event.Status)

}

func (e *JobExecutor) ActiveJobs() uint32 {
	return atomic.LoadUint32(&e.activeJobs)
}

func (e *JobExecutor) stepQueueRoutine(ctx context.Context, stepExecutor *step.Executor) {
	runStepQueue(ctx, stepExecutor, func(jobCtx *job.Context) {
		stepExecutor.OnJob(jobCtx)
		err := e.executeStepActions(ctx, stepExecutor, jobCtx)
		if err != nil {
			stepExecutor.OnError(jobCtx, stepExecutor.NotificationType(), err)
			return
		}
		stepExecutor.OnComplete(jobCtx)
	})
}

func runStepQueue(ctx context.Context, stepExecutor *step.Executor, process func(*job.Context)) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobCtx, ok := <-stepExecutor.GetJobChan():
			if !ok {
				return
			}
			process(jobCtx)
		}
	}
}

func (e *JobExecutor) executeStepActions(ctx context.Context, stepExecutor *step.Executor, jobContext *job.Context) error {
	wg := sync.WaitGroup{}

	actions := stepExecutor.Actions(jobContext)
	actionsChan := make(chan step.Action, len(actions))
	errs := make(chan error, len(actions))

	// prepare parallel runners
	for i := 0; i < stepExecutor.Parallel(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for action := range actionsChan {
				tracker := e.ReportTrackStep(jobContext.JobId, action.Id, stepExecutor.NotificationType())
				if err := action.Execute(ctx, tracker); err != nil {
					tracker.Logger().Errorf("Error on executing step %s: %s", stepExecutor.NotificationType(), err.Error())
					tracker.Error()
					errs <- err
					continue
				}
				tracker.Done()
			}
		}()
	}

	e.publishTaskEvent(jobContext, stepExecutor.NotificationType(), model.StartedNotificationStatus, "")
	for _, action := range actions {
		actionsChan <- action
	}

	close(actionsChan)
	wg.Wait()
	close(errs)

	var errorList []error
	for err := range errs {
		errorList = append(errorList, err)
	}
	err := errors.Join(errorList...)
	if err != nil {
		e.publishTaskEvent(jobContext, stepExecutor.NotificationType(), model.FailedNotificationStatus, err.Error())
		return err
	}

	e.publishTaskEvent(jobContext, stepExecutor.NotificationType(), model.CompletedNotificationStatus, "")
	return nil
}

func (e *JobExecutor) CleanJob(jobContext *job.Context) error {
	atomic.AddUint32(&e.activeJobs, ^uint32(0))
	return jobContext.Clean()
}

// ConsoleTrackStep Console tracker to print progress to console
func (e *JobExecutor) ConsoleTrackStep(stepId string, notificationType model.NotificationType) *console.StepTracker {
	return e.console.StepTracker(stepId, notificationType, e.console.Logger(console.WithMessagePrefix(fmt.Sprintf("[%s]", stepId))))
}

// ReportTrackStep This one is like consoleTrackStep but also reports progress to server
func (e *JobExecutor) ReportTrackStep(jobId uuid.UUID, stepId string, notificationType model.NotificationType) *ReportStepProgressTracker {
	consoleTracker := e.ConsoleTrackStep(stepId, notificationType)
	return newReportStepProgressTracker(jobId, stepId, notificationType, e.client, consoleTracker)
}
