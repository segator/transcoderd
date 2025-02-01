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

const maxPrefetchedJobs = 1

type JobExecutor struct {
	prefetchJobs uint32
	stepChan     chan []*job.Context

	workerConfig *config.Config
	tempPath     string
	wg           sync.WaitGroup
	mu           sync.RWMutex
	client       *serverclient.ServerClient

	console *console.RenderService
}

func NewEncodeWorker(workerConfig *config.Config, client *serverclient.ServerClient, renderService *console.RenderService) *JobExecutor {
	tempPath := filepath.Join(workerConfig.TemporalPath, fmt.Sprintf("worker-%s", workerConfig.Name))

	encodeWorker := &JobExecutor{
		client:       client,
		wg:           sync.WaitGroup{},
		workerConfig: workerConfig,
		stepChan:     make(chan *job.Context, 100),
		tempPath:     tempPath,
		console:      renderService,
		prefetchJobs: 0,
	}
	if err := os.MkdirAll(tempPath, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	return encodeWorker
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
	go e.downloadQueueRoutine(ctx)
	go e.encodeQueueRoutine(ctx)
	go e.uploadQueueRoutine(ctx)
}

func (e *JobExecutor) stop() {
	defer close(e.downloadChan)
	defer close(e.uploadChan)
	defer close(e.encodeChan)
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

			switch jobContext.LastEvent.NotificationType {
			case model.DownloadNotification:
				e.AddDownloadJob(jobContext)
				// TODO esto esta mal, el encode tiene varios steps
			case model.FFMPEGSNotification:
				// add as prefetched job so won't try to download more jobs until jobs are in encoding phase
				atomic.AddUint32(&e.prefetchJobs, 1)
				e.AddEncodeJob(jobContext)
			case model.UploadNotification:
				e.AddUploadJob(jobContext)
			default:
				log.Panicf("if this happens is a bug %s", jobContext.LastEvent.NotificationType)
			}
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
	return e.PrefetchJobs() < maxPrefetchedJobs
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
	e.AddDownloadJob(jobContext)
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
	l.Logf("%s have been %s: %s", event.NotificationType, event.Status, event.Message)

}

func (e *JobExecutor) PrefetchJobs() uint32 {
	return atomic.LoadUint32(&e.prefetchJobs)
}

func (e *JobExecutor) AddDownloadJob(job *job.Context) {
	atomic.AddUint32(&e.prefetchJobs, 1)
	e.downloadChan <- job
}

func (e *JobExecutor) AddEncodeJob(job *job.Context) {
	e.encodeChan <- job
}

func (e *JobExecutor) AddUploadJob(job *job.Context) {
	e.uploadChan <- job
}

func (e *JobExecutor) downloadQueueRoutine(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()
	downloadStepExecutor := step.NewDownloadStepExecutor(e.workerConfig.Name, e.client.GetBaseDomain())
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-e.downloadChan:
			if !ok {
				return
			}

			stepFunc := func(ctx context.Context, stepTracker step.Tracker) error {
				videoData, err := downloadStepExecutor.Execute(ctx, stepTracker, job)
				if err == nil {
					job.Source = videoData
				}
				return err
			}

			err := e.executeSingleStep(ctx, stepFunc, job, model.DownloadNotification)
			if err != nil {
				atomic.AddUint32(&e.prefetchJobs, ^uint32(0))
				continue
			}
			e.AddEncodeJob(job)
		}
	}

}

type StepAction struct {
	Execute StepActionFunc
	Id      string
}
type StepActionFunc func(ctx context.Context, stepTracker step.Tracker) error

func (e *JobExecutor) executeSingleStep(ctx context.Context, stepActionFunc StepActionFunc, jobContext *job.Context, notificationType model.NotificationType) error {
	stepActions := []StepAction{
		{
			Execute: stepActionFunc, Id: jobContext.JobId.String(),
		},
	}
	return e.executeParallelStep(ctx, 1, stepActions, jobContext, notificationType)
}

func (e *JobExecutor) executeParallelStep(ctx context.Context, parallelSteps int, stepActions []StepAction, jobContext *job.Context, notificationType model.NotificationType) error {
	wg := sync.WaitGroup{}
	actionsChan := make(chan StepAction, len(stepActions))
	errs := make(chan error, len(stepActions))

	for i := 0; i < parallelSteps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for action := range actionsChan {
				tracker := e.TrackStep(jobContext.JobId, action.Id, notificationType)
				tracker.SetTotal(0)
				if err := action.Execute(ctx, tracker); err != nil {
					tracker.Logger().Errorf("Error on executing step %s: %s", notificationType, err.Error())
					tracker.Error()
					errs <- err
					continue
				}
				tracker.Done()
			}
		}()
	}

	e.publishTaskEvent(jobContext, notificationType, model.StartedNotificationStatus, "")
	for _, action := range stepActions {
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
		e.publishTaskEvent(jobContext, notificationType, model.FailedNotificationStatus, err.Error())
		if err := jobContext.Clean(); err != nil {
			e.jobLogger(jobContext).Errorf("failed to clean job workspace %v", err)
		}
		return err
	}

	e.publishTaskEvent(jobContext, notificationType, model.CompletedNotificationStatus, "")
	return nil
}

func (e *JobExecutor) uploadQueueRoutine(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()
	uploadStepExecutor := step.NewUploadStepExecutor(e.workerConfig.Name, e.client.GetBaseDomain())
	for {
		select {
		case <-ctx.Done():
			return
		case jobContext, ok := <-e.uploadChan:
			if !ok {
				continue
			}
			uploadStepFunc := func(ctx context.Context, stepTracker step.Tracker) error {
				return uploadStepExecutor.Execute(ctx, stepTracker, jobContext)
			}
			if err := e.executeSingleStep(ctx, uploadStepFunc, jobContext, model.UploadNotification); err != nil {
				continue
			}

			e.publishTaskEvent(jobContext, model.JobNotification, model.CompletedNotificationStatus, "")
			if err := jobContext.Clean(); err != nil {
				e.jobLogger(jobContext).Errorf("failed to clean job workspace: %v", err)
			}
		}
	}

}

func (e *JobExecutor) encodeQueueRoutine(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()

	mkvExtractStepExecutor := step.NewMKVExtractStepExecutor()
	ffmpegStepExecutor := step.NewFFMPEGStepExecutor(e.workerConfig.EncodeConfig)
	pgsStepExecutor := step.NewPGSToSrtStepExecutor(e.workerConfig.PGSConfig)
	ffmpegVerifyStep := step.NewFFMPEGVerifyStepExecutor(e.workerConfig.VerifyDeltaTime)
	for {
		select {
		case <-ctx.Done():
			return
		case jobContext, ok := <-e.encodeChan:
			if !ok {
				return
			}
			atomic.AddUint32(&e.prefetchJobs, ^uint32(0))

			if jobContext.Source.FFProbeData.HaveImageTypeSubtitle() {
				mkvExtractStepFunc := func(ctx context.Context, stepTracker step.Tracker) error {
					return mkvExtractStepExecutor.Execute(ctx, stepTracker, jobContext)
				}

				if err := e.executeSingleStep(ctx, mkvExtractStepFunc, jobContext, model.MKVExtractNotification); err != nil {
					continue
				}

				var pgsStepActions []StepAction
				for _, pgs := range jobContext.Source.FFProbeData.GetPGSSubtitles() {
					pgsStepActions = append(pgsStepActions, StepAction{
						Execute: func(ctx context.Context, stepTracker step.Tracker) error {
							return pgsStepExecutor.Execute(ctx, stepTracker, jobContext, pgs)
						},
						Id: fmt.Sprintf("%s %d", jobContext.JobId, pgs.Id),
					})
				}
				if err := e.executeParallelStep(ctx, e.workerConfig.PGSConfig.ParallelJobs, pgsStepActions, jobContext, model.PGSNotification); err != nil {
					continue
				}
			}
			ffmpegStepFunc := func(ctx context.Context, stepTracker step.Tracker) error {
				return ffmpegStepExecutor.Execute(ctx, stepTracker, jobContext)
			}
			if err := e.executeSingleStep(ctx, ffmpegStepFunc, jobContext, model.FFMPEGSNotification); err != nil {
				continue
			}

			ffmpegVerifyFunc := func(ctx context.Context, stepTracker step.Tracker) error {
				return ffmpegVerifyStep.Execute(jobContext)
			}
			if err := e.executeSingleStep(ctx, ffmpegVerifyFunc, jobContext, model.JobVerify); err != nil {
				continue
			}
			e.AddUploadJob(jobContext)
		}
	}
}

func (e *JobExecutor) TrackStep(jobId uuid.UUID, stepId string, notificationType model.NotificationType) *ReportStepProgressTracker {
	stepLogger := e.console.Logger(console.WithMessagePrefix(fmt.Sprintf("[%s]", stepId)))
	consoleTracker := e.console.StepTracker(stepId, notificationType, stepLogger)
	return newReportStepProgressTracker(jobId, stepId, notificationType, e.client, consoleTracker)

}
