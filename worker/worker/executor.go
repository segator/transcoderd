package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"
)

const maxActiveJobs = 2

type JobExecutor struct {
	activeJobs uint32

	workerConfig *config.Config
	tempPath     string
	wg           sync.WaitGroup
	mu           sync.RWMutex
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
			if jobContext.Source.FFProbeData.HaveImageTypeSubtitle() {
				stepExecutors[model.MKVExtractNotification].AddJob(jobContext)
				return
			}
			stepExecutors[model.FFMPEGSNotification].AddJob(jobContext)
		}),
		onErrOpt)

	// MKVExtract Step
	stepExecutors[model.MKVExtractNotification] = step.NewMKVExtractStepExecutor(onErrOpt,
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			stepExecutors[model.PGSNotification].AddJob(jobContext)
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
	for _, stepExecutor := range e.stepExecutors {
		go e.stepQueueRoutine(ctx, stepExecutor)
	}
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
			filepath.Base(path)
			jobContext := job.ReadContextFromDiskByPath(path)
			atomic.AddUint32(&e.activeJobs, 1)

			e.stepExecutors[jobContext.LastEvent.NotificationType].AddJob(jobContext)
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (e *JobExecutor) AcceptJobs() bool {
	if e.workerConfig.Paused {
		return false
	}
	if e.workerConfig.HaveSettedPeriodTime() {
		now := time.Now()
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		elapsedSinceMidnight := now.Sub(midnight)
		return elapsedSinceMidnight >= *e.workerConfig.StartAfter && elapsedSinceMidnight <= *e.workerConfig.StopAfter
	}
	return e.ActiveJobs() < maxActiveJobs
}
func (e *JobExecutor) jobLogger(jobContext *job.Context) console.LeveledLogger {
	return e.console.Logger(console.WithMessagePrefix(fmt.Sprintf("[%s]", jobContext.JobId.String())))
}
func (e *JobExecutor) ExecuteJob(jobId uuid.UUID, lastEvent int) error {
	jobContext := job.NewContext(jobId, lastEvent, filepath.Join(e.tempPath, jobId.String()))

	if err := jobContext.Init(); err != nil {
		return err
	}

	e.publishTaskEvent(jobContext, model.JobNotification, model.StartedNotificationStatus, "")

	atomic.AddUint32(&e.activeJobs, 1)
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
	for {
		select {
		case <-ctx.Done():
			return
		case jobCtx, ok := <-stepExecutor.GetJobChan():
			if !ok {
				return
			}
			stepExecutor.OnJob(jobCtx)
			err := e.executeStepActions(ctx, stepExecutor, jobCtx)
			if err != nil {
				stepExecutor.OnError(jobCtx, stepExecutor.NotificationType(), err)
				continue
			}
			stepExecutor.OnComplete(jobCtx)
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
