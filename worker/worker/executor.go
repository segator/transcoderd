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
	activeJobs      uint32
	reservedJobs    int
	maxActiveJobs   int
	settingsPending bool
	admissionHeld   bool

	workerConfig *config.Config
	tempPath     string
	wg           sync.WaitGroup
	mu           sync.RWMutex
	admissionMu  sync.Mutex
	settingsMu   sync.Mutex
	progress     progressRegistry
	jobs         jobRegistry
	client       *serverclient.ServerClient
	encodePool   *encodeConcurrency

	console       *console.RenderService
	stepExecutors map[model.NotificationType]*step.Executor
}

// pipelineBufferJobs keeps one job available for encoding while allowing a
// recently encoded job to finish verification/upload. The buffer is bounded
// so slower downstream stages cannot grow the worker's queue without limit.
const pipelineBufferJobs = 2

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
		encodePool:    newEncodeConcurrency(workerConfig.EncodeParallelJobs()),
		jobs:          jobRegistry{codec: workerConfig.EncodeConfig},
	}
	stepExecutors := setupStepExecutors(jobExecutor)
	jobExecutor.trackPipeline(stepExecutors)
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
		if notificationType == model.FFMPEGSNotification {
			go e.encodeQueueRoutine(ctx, stepExecutor)
		} else {
			go e.stepQueueRoutine(ctx, stepExecutor)
		}
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
	var activeEncodes int
	maxEncodes := e.maxActiveJobs
	if e.encodePool != nil {
		activeEncodes, maxEncodes = e.encodePool.Status()
	}
	maxAccepted := 0
	if e.maxActiveJobs > 0 {
		maxAccepted = maxAcceptedJobs(maxEncodes)
	}
	return AdmissionStatus{
		Accepting:       !e.admissionHeld && !e.settingsPending && e.maxActiveJobs > 0 && withinSchedule,
		ActiveJobs:      e.ActiveJobs(),
		ReservedJobs:    e.reservedJobs,
		MaxActiveJobs:   e.maxActiveJobs,
		MaxAcceptedJobs: maxAccepted,
		ActiveEncodes:   activeEncodes,
		MaxEncodes:      maxEncodes,
		WithinSchedule:  withinSchedule,
		Progress:        e.progress.snapshot(),
		Jobs:            e.jobs.snapshot(),
	}
}

func (e *JobExecutor) SetMaxActiveJobs(max int) error {
	if max < 0 {
		return fmt.Errorf("maximum active jobs must be non-negative")
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.maxActiveJobs = max
	if max > 0 && e.encodePool != nil {
		e.encodePool.SetTarget(max)
	}
	return nil
}

func (e *JobExecutor) PersistMaxActiveJobs(max int) error {
	if max < 0 {
		return fmt.Errorf("maximum active jobs must be non-negative")
	}
	if e.client == nil {
		return errors.New("worker settings cannot be saved without a server client")
	}
	e.settingsMu.Lock()
	defer e.settingsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.client.SetWorkerMaxActiveJobs(ctx, max); err != nil {
		return fmt.Errorf("persist maximum active jobs: %w", err)
	}
	if err := e.SetMaxActiveJobs(max); err != nil {
		return err
	}
	e.MarkWorkerSettingsResolved()
	return nil
}

func (e *JobExecutor) RequireWorkerSettings() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.settingsPending = true
}

// HoldAdmission prevents persisted server settings from reopening requests during
// a managed installation update. Accepted pipeline work continues normally.
func (e *JobExecutor) HoldAdmission() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.admissionHeld = true
}

func (e *JobExecutor) MarkWorkerSettingsResolved() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.settingsPending = false
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
	if e.admissionHeld || e.settingsPending || e.maxActiveJobs <= 0 || !e.workerConfig.AdmissionAllowedAt(now) {
		return false
	}
	encodeTarget := e.maxActiveJobs
	if e.encodePool != nil {
		_, encodeTarget = e.encodePool.Status()
	}
	return uint64(e.ActiveJobs())+uint64(e.reservedJobs) < uint64(maxAcceptedJobs(encodeTarget))
}

func maxAcceptedJobs(encodeTarget int) int {
	if encodeTarget <= 0 {
		return 0
	}
	maxInt := int(^uint(0) >> 1)
	if encodeTarget > maxInt-pipelineBufferJobs {
		return maxInt
	}
	return encodeTarget + pipelineBufferJobs
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
	return e.ExecuteReservedJob(jobId, lastEvent, "")
}

func (e *JobExecutor) ExecuteReservedJob(jobId uuid.UUID, lastEvent int, originalName string) error {
	jobContext := job.NewContext(jobId, lastEvent, filepath.Join(e.tempPath, jobId.String()))
	jobContext.Source.OriginalName = originalName

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
		e.processStep(ctx, stepExecutor, jobCtx)
	})
}

func (e *JobExecutor) encodeQueueRoutine(ctx context.Context, stepExecutor *step.Executor) {
	runDynamicStepQueue(ctx, stepExecutor, e.encodePool, func(jobCtx *job.Context) {
		e.processStep(ctx, stepExecutor, jobCtx)
	})
}

func (e *JobExecutor) processStep(ctx context.Context, stepExecutor *step.Executor, jobCtx *job.Context) {
	stepExecutor.OnJob(jobCtx)
	err := e.executeStepActions(ctx, stepExecutor, jobCtx)
	if err != nil {
		stepExecutor.OnError(jobCtx, stepExecutor.NotificationType(), err)
		return
	}
	stepExecutor.OnComplete(jobCtx)
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
				tracker := e.ReportTrackStep(jobContext, action.Id, stepExecutor.NotificationType())
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
	err := e.cleanJob(jobContext.Clean)
	e.jobs.remove(jobContext.JobId.String())
	return err
}

func (e *JobExecutor) cleanJob(cleanup func() error) error {
	err := cleanup()
	atomic.AddUint32(&e.activeJobs, ^uint32(0))
	return err
}

// ConsoleTrackStep Console tracker to print progress to console
func (e *JobExecutor) ConsoleTrackStep(stepId string, notificationType model.NotificationType) *console.StepTracker {
	return e.console.StepTracker(stepId, notificationType, e.console.Logger(console.WithMessagePrefix(fmt.Sprintf("[%s]", stepId))))
}

// ReportTrackStep This one is like consoleTrackStep but also reports progress to server
func (e *JobExecutor) ReportTrackStep(jobContext *job.Context, stepId string, notificationType model.NotificationType) *ReportStepProgressTracker {
	consoleTracker := e.ConsoleTrackStep(stepId, notificationType)
	codec := ""
	if e.workerConfig != nil && e.workerConfig.EncodeConfig != nil {
		codec = e.workerConfig.EncodeConfig.VideoCodec
	}
	tracker := newReportStepProgressTracker(jobContext.JobId, stepId, notificationType, codec, e.client, consoleTracker)
	if jobContext.Source != nil {
		tracker.name = jobContext.Source.OriginalName
	}
	tracker.registry = &e.progress
	tracker.updateSnapshot()
	return tracker
}
