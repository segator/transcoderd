package worker

import (
	"github.com/google/uuid"
	"math"
	"sort"
	"sync"
	"time"
	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/control"
	"transcoder/worker/serverclient"
)

type ReportStepProgressTracker struct {
	notificationType   model.NotificationType
	consoleStepTracker *console.StepTracker
	serverClient       *serverclient.ServerClient
	logger             console.LeveledLogger
	jobId              uuid.UUID
	name               string
	stepId             string
	lastUpdate         time.Time
	registry           *progressRegistry
	snapshotMu         sync.Mutex
	reportMu           sync.Mutex
	finished           bool
}

type progressRegistry struct {
	mu    sync.RWMutex
	items map[string]control.TaskProgress
}

func (r *progressRegistry) set(key string, value control.TaskProgress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]control.TaskProgress)
	}
	r.items[key] = value
}

func (r *progressRegistry) remove(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, key)
}

func (r *progressRegistry) snapshot() []control.TaskProgress {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]control.TaskProgress, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].JobID != items[j].JobID {
			return items[i].JobID < items[j].JobID
		}
		return items[i].StepID < items[j].StepID
	})
	return items
}

func (e *ReportStepProgressTracker) snapshotKey() string {
	return e.jobId.String() + "\x00" + string(e.notificationType) + "\x00" + e.stepId
}

func (e *ReportStepProgressTracker) updateSnapshot() {
	e.snapshotMu.Lock()
	defer e.snapshotMu.Unlock()
	if e.finished {
		return
	}
	if e.registry == nil {
		return
	}
	percent := e.consoleStepTracker.PercentDone()
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	eta := int64(e.consoleStepTracker.ETA().Seconds())
	if eta < 0 {
		eta = 0
	}
	e.registry.set(e.snapshotKey(), control.TaskProgress{JobID: e.jobId.String(), Name: e.name, StepID: e.stepId, Phase: string(e.notificationType), Percent: percent, ETASeconds: eta})
}

func (e *ReportStepProgressTracker) finishSnapshot() {
	e.snapshotMu.Lock()
	defer e.snapshotMu.Unlock()
	e.finished = true
	if e.registry != nil {
		e.registry.remove(e.snapshotKey())
	}
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

// SetDisplayName lets a download reveal its filename as soon as headers arrive.
func (e *ReportStepProgressTracker) SetDisplayName(name string) {
	e.snapshotMu.Lock()
	e.name = name
	e.snapshotMu.Unlock()
	e.updateSnapshot()
}

func (e *ReportStepProgressTracker) SetTotal(total int64) {
	e.consoleStepTracker.SetTotal(total)
	e.updateSnapshot()
}

func (e *ReportStepProgressTracker) UpdateValue(value int64) {
	e.consoleStepTracker.UpdateValue(value)
	e.updateSnapshot()
	e.reportTrackProgress(model.ProgressingTaskProgressTypeStatus)
}

func (e *ReportStepProgressTracker) Increment(increment int) {
	e.consoleStepTracker.Increment(increment)
	e.updateSnapshot()
	e.reportTrackProgress(model.ProgressingTaskProgressTypeStatus)
}

func (e *ReportStepProgressTracker) reportTrackProgress(status model.TaskProgressStatus) {
	e.reportMu.Lock()
	defer e.reportMu.Unlock()
	if status == model.ProgressingTaskProgressTypeStatus {
		e.snapshotMu.Lock()
		finished := e.finished
		e.snapshotMu.Unlock()
		if finished {
			return
		}
	}
	if time.Since(e.lastUpdate) > 15*time.Second || status != model.ProgressingTaskProgressTypeStatus {
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
	e.finishSnapshot()
	e.reportTrackProgress(model.FailureTaskProgressTypeStatus)
}

func (e *ReportStepProgressTracker) Done() {
	e.consoleStepTracker.Done()
	e.finishSnapshot()
	e.reportTrackProgress(model.DoneTaskProgressTypeStatus)
}
