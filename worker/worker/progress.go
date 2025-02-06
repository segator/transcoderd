package worker

import (
	"github.com/google/uuid"
	"time"
	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/serverclient"
)

type ReportStepProgressTracker struct {
	notificationType   model.NotificationType
	consoleStepTracker *console.StepTracker
	serverClient       *serverclient.ServerClient
	logger             console.LeveledLogger
	jobId              uuid.UUID
	stepId             string
	lastUpdate         time.Time
}

func newReportStepProgressTracker(jobId uuid.UUID, stepId string, notificationType model.NotificationType, serverClient *serverclient.ServerClient, consoleStepTracker *console.StepTracker) *ReportStepProgressTracker {
	return &ReportStepProgressTracker{
		jobId:              jobId,
		stepId:             stepId,
		serverClient:       serverClient,
		notificationType:   notificationType,
		consoleStepTracker: consoleStepTracker,
		logger:             consoleStepTracker.Logger(),
	}
}
func (e *ReportStepProgressTracker) Logger() console.LeveledLogger {
	return e.logger
}

func (e *ReportStepProgressTracker) SetTotal(total int64) {
	e.consoleStepTracker.SetTotal(total)
}

func (e *ReportStepProgressTracker) UpdateValue(value int64) {
	e.consoleStepTracker.UpdateValue(value)
	e.reportTrackProgress(model.ProgressingTaskProgressTypeStatus)
}

func (e *ReportStepProgressTracker) Increment(increment int) {
	e.consoleStepTracker.Increment(increment)
	e.reportTrackProgress(model.ProgressingTaskProgressTypeStatus)
}

func (e *ReportStepProgressTracker) reportTrackProgress(status model.TaskProgressStatus) {
	if time.Since(e.lastUpdate) > 5*time.Second || status != model.ProgressingTaskProgressTypeStatus {
		err := e.serverClient.PublishTaskProgressEvent(&model.TaskProgressType{
			Event: model.Event{
				EventTime: time.Now(),
			},
			JobId:            e.jobId,
			ProgressID:       e.stepId,
			Percent:          e.consoleStepTracker.PercentDone(),
			ETA:              e.consoleStepTracker.ETA(),
			NotificationType: e.notificationType,
			Status:           status,
		})
		if err != nil {
			e.logger.Errorf("Error on publishing track progress %s", err.Error())
		}
		e.lastUpdate = time.Now()
	}
}

func (e *ReportStepProgressTracker) Error() {
	e.consoleStepTracker.Error()
	e.reportTrackProgress(model.FailureTaskProgressTypeStatus)
}

func (e *ReportStepProgressTracker) Done() {
	e.consoleStepTracker.Done()
	e.reportTrackProgress(model.DoneTaskProgressTypeStatus)
}
