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

	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/control"
	"transcoder/worker/job"
	"transcoder/worker/resources"
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
	finishing       bool
	managedLimits   bool

	serviceCtx      context.Context
	workerConfig    *config.Config
	tempPath        string
	wg              sync.WaitGroup
	admissionMu     sync.Mutex
	settingsMu      sync.Mutex
	progress        progressRegistry
	jobs            jobRegistry
	client          *serverclient.ServerClient
	encodePool      *encodeConcurrency
	encodeProcesses *command.PauseController
	pauseMode       string
	ramPaused       bool
	resources       resourceSampler

	console       *console.RenderService
	stepExecutors map[model.NotificationType]*step.Executor
}

type resourceSampler interface {
	Start(context.Context)
	Stop()
	Snapshot() *model.WorkerResources
}

// One extra accepted job overlaps preparation or upload without accumulating
// multiple prefetched movies for each encoder profile.
const pipelineBufferJobs = 1

func NewEncodeWorker(workerConfig *config.Config, client *serverclient.ServerClient, renderService *console.RenderService) *JobExecutor {
	tempPath := filepath.Join(workerConfig.TemporalPath, fmt.Sprintf("worker-%s", workerConfig.Name))

	jobExecutor := &JobExecutor{
		client:          client,
		wg:              sync.WaitGroup{},
		workerConfig:    workerConfig,
		stepExecutors:   make(map[model.NotificationType]*step.Executor),
		tempPath:        tempPath,
		console:         renderService,
		activeJobs:      0,
		maxActiveJobs:   workerConfig.MaxActiveJobs,
		encodePool:      newEncodeConcurrency(workerConfig.EncodeParallelJobs()),
		encodeProcesses: &command.PauseController{},
		pauseMode:       workerConfig.PauseMode,
		ramPaused:       workerConfig.RAMPaused,
		jobs:            jobRegistry{codec: workerConfig.EncodeConfig},
		resources:       resources.NewSampler(),
	}
	if jobExecutor.pauseMode == "" {
		jobExecutor.pauseMode = config.PauseFinish
	}
	if workerConfig.RAMPaused || (workerConfig.MaxActiveJobs == 0 && jobExecutor.pauseMode == config.PauseRAM) {
		_ = jobExecutor.encodeProcesses.SetPaused(true)
	}
	jobExecutor.encodePool.SetPaused(workerConfig.MaxActiveJobs == 0 || workerConfig.RAMPaused)
	stepExecutors := setupStepExecutors(jobExecutor)
	jobExecutor.trackPipeline(stepExecutors)
	jobExecutor.stepExecutors = stepExecutors
	jobExecutor.encodePool.setPrimary(config.EncoderKind(workerConfig.EncodeConfig.VideoCodec))
	if workerConfig.MaxActiveJobs == 0 {
		jobExecutor.encodePool.pause()
	}

	if err := os.MkdirAll(tempPath, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	return jobExecutor
}

func setupStepExecutors(jobExecutor *JobExecutor) map[model.NotificationType]*step.Executor {
	workerConfig := jobExecutor.workerConfig
	client := jobExecutor.client

	onErrOpt := step.WithOnErrorOpt(func(jobContext *job.Context, notificationType model.NotificationType, err error) {
		if err := jobExecutor.publishTaskEvent(jobContext, model.JobNotification, model.FailedNotificationStatus, fmt.Sprintf("%s:%s", notificationType, err.Error())); err != nil {
			jobExecutor.handleDeliveryError(jobContext, err)
			return
		}
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
		step.WithPauseController(jobExecutor.encodeProcesses),
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
			if err := jobExecutor.publishTaskEvent(jobContext, model.JobNotification, model.CompletedNotificationStatus, ""); err != nil {
				jobExecutor.handleDeliveryError(jobContext, err)
				return
			}
			jobExecutor.ConsoleTrackStep(jobContext.JobId.String(), model.JobNotification).Done()
			if err := jobExecutor.CleanJob(jobContext); err != nil {
				jobExecutor.jobLogger(jobContext).Errorf("failed to clean job workspace: %v", err)
			}
		}),
		onErrOpt)

	return stepExecutors
}

func (e *JobExecutor) Run(wg *sync.WaitGroup, ctx context.Context) error {
	e.serviceCtx = ctx
	if err := e.resumeJobs(ctx); err != nil {
		return err
	}
	serviceCtx, cancelServiceCtx := context.WithCancel(context.Background())
	e.serviceCtx = serviceCtx
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
	return nil
}

func (e *JobExecutor) start(ctx context.Context) {
	if e.resources != nil {
		e.resources.Start(ctx)
	}
	for notificationType, stepExecutor := range e.stepExecutors {
		if notificationType == model.FFMPEGSNotification {
			go e.encodeQueueRoutine(ctx, stepExecutor)
		} else {
			go e.stepQueueRoutine(ctx, stepExecutor)
		}
	}
}

func (e *JobExecutor) stop() {
	if e.resources != nil {
		e.resources.Stop()
	}
	// Queue consumers exit on context cancellation. Closing their channels
	// while in-flight actions can still enqueue a successor would race.
}

func (e *JobExecutor) resumeJobs(ctx context.Context) error {
	return filepath.Walk(e.tempPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
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
			// Persisted paths are not trusted: recovery may delete a workspace
			// only when it is the canonical job directory under this worker.
			workspace, workspaceErr := filepath.Abs(filepath.Join(e.tempPath, jobContext.JobId.String()))
			savedDir, savedErr := filepath.Abs(jobContext.WorkingDir)
			snapshot, snapshotErr := filepath.Abs(path)
			if jobContext.JobId == uuid.Nil || workspaceErr != nil || savedErr != nil || snapshotErr != nil ||
				savedDir != workspace || filepath.Dir(snapshot) != workspace || filepath.Base(snapshot) != jobContext.JobId.String()+".json" {
				log.WithField("context_path", path).Warn("Ignoring persisted job context with invalid workspace")
				return nil
			}
			if jobContext.RejectedReason != "" {
				log.WithFields(log.Fields{"job_id": jobContext.JobId.String(), "reason": jobContext.RejectedReason}).Error("Preserving rejected job for inspection")
				return nil
			}
			pending := jobContext.PendingLegacyEvent()
			if jobContext.LastEvent == nil && pending == nil {
				log.WithField("context_path", path).Warn("Ignoring persisted job context without a last event")
				return nil
			}
			if pending == nil && jobContext.LastEvent != nil && jobContext.LastEvent.NotificationType == model.JobNotification {
				switch jobContext.LastEvent.Status {
				case model.CompletedNotificationStatus, model.FailedNotificationStatus, model.CanceledNotificationStatus:
					if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
						return err
					}
					return nil
				}
			}
			if jobContext.LastEvent != nil && jobContext.LastEvent.NotificationType != model.JobNotification {
				if _, ok := e.stepExecutors[jobContext.LastEvent.NotificationType]; !ok {
					log.WithField("context_path", path).Warn("Ignoring persisted job context with unsupported phase")
					return nil
				}
			}
			current, err := e.client.GetJobForRecovery(ctx, jobContext.JobId)
			if err != nil {
				return err
			}
			// A completed terminal event may have been committed while its
			// response was lost. Replaying it performs any remaining cleanup.
			terminalPending := pending != nil && pending.IsCompleted() && current != nil &&
				current.Events.GetLatestPerNotificationType(model.JobNotification) != nil &&
				current.Events.GetLatestPerNotificationType(model.JobNotification).EventID == pending.EventID
			if current == nil || (!terminalPending && !e.matchesSavedAssignment(current, jobContext)) {
				log.WithField("job_id", jobContext.JobId.String()).Info("Discarding expired local assignment")
				workspace := filepath.Join(e.tempPath, jobContext.JobId.String())
				if path == filepath.Join(workspace, jobContext.JobId.String()+".json") {
					if err := os.RemoveAll(workspace); err != nil {
						log.WithError(err).WithField("context_path", path).Warn("Could not remove expired workspace")
					}
					return filepath.SkipDir
				}
				return nil
			}
			if pending == nil && jobContext.LastEvent != nil && current.Events.GetLatest() != nil {
				// Compatibility with snapshots written before the durable outbox.
				jobContext.LastEvent.EventId = current.Events.GetLatest().EventID
			}
			if pending != nil {
				if err := e.client.PublishTaskEventContext(ctx, pending); err != nil {
					if errors.Is(err, serverclient.ErrAssignmentExpired) {
						if err := jobContext.Clean(); err != nil {
							return err
						}
						return filepath.SkipDir
					}
					if errors.Is(err, serverclient.ErrNotificationConflict) {
						return jobContext.MarkRejected(err.Error())
					}
					return err
				}
				if err := jobContext.AcknowledgeLegacyEvent(pending.EventID); err != nil {
					return err
				}
			}
			if jobContext.LastEvent == nil {
				return fmt.Errorf("job %s has no acknowledged event", jobContext.JobId)
			}
			notificationType := jobContext.LastEvent.NotificationType
			status := jobContext.LastEvent.Status
			switch {
			case notificationType == model.JobNotification:
				switch status {
				case model.AssignedNotificationStatus, model.StartedNotificationStatus:
					notificationType = model.DownloadNotification
				case model.CompletedNotificationStatus, model.FailedNotificationStatus, model.CanceledNotificationStatus:
					if err := jobContext.Clean(); err != nil {
						return err
					}
					return filepath.SkipDir
				default:
					return fmt.Errorf("job %s has unsupported terminal state %s", jobContext.JobId, status)
				}
			case status == model.FailedNotificationStatus:
				if err := e.publishTaskEvent(jobContext, model.JobNotification, model.FailedNotificationStatus,
					fmt.Sprintf("%s:%s", notificationType, jobContext.LastEvent.Message)); err != nil {
					return err
				}
				if err := jobContext.Clean(); err != nil {
					return err
				}
				return filepath.SkipDir
			case status == model.CompletedNotificationStatus:
				switch notificationType {
				case model.DownloadNotification:
					if jobContext.Source.FFProbeData.HaveExtractableSubtitle() {
						notificationType = model.MKVExtractNotification
					} else {
						notificationType = model.FFMPEGSNotification
					}
				case model.MKVExtractNotification:
					if jobContext.Source.FFProbeData.HaveImageTypeSubtitle() {
						notificationType = model.PGSNotification
					} else {
						notificationType = model.FFMPEGSNotification
					}
				case model.PGSNotification:
					notificationType = model.FFMPEGSNotification
				case model.FFMPEGSNotification:
					notificationType = model.JobVerify
				case model.JobVerify:
					notificationType = model.UploadNotification
				case model.UploadNotification:
					if err := e.publishTaskEvent(jobContext, model.JobNotification, model.CompletedNotificationStatus, ""); err != nil {
						return err
					}
					if err := jobContext.Clean(); err != nil {
						return err
					}
					return filepath.SkipDir
				}
			}
			stepExecutor, ok := e.stepExecutors[notificationType]
			if !ok || stepExecutor == nil {
				return fmt.Errorf("job %s has unsupported phase %s", jobContext.JobId, notificationType)
			}

			atomic.AddUint32(&e.activeJobs, 1)
			stepExecutor.AddJob(jobContext)
		}

		return nil
	})
}

func (e *JobExecutor) matchesSavedAssignment(current *model.Job, saved *job.Context) bool {
	started := current.Events.GetLatestPerNotificationType(model.JobNotification)
	return started != nil && started.WorkerName == e.workerConfig.Name &&
		((started.Status == model.StartedNotificationStatus && started.EventID == saved.EventId+1) ||
			(started.Status == model.AssignedNotificationStatus && started.EventID == saved.EventId && saved.PendingLegacyEvent() != nil))
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
	var cpuJobs, gpuJobs, cpuActive, gpuActive, cpuTarget, gpuTarget int
	independent := false
	if e.encodePool != nil {
		cpuJobs, gpuJobs, cpuActive, gpuActive, cpuTarget, gpuTarget = e.encodePool.encoderStatus()
	}
	if e.encodePool != nil {
		independent = e.encodePool.hasIndependentLimits()
	}
	maxAccepted := 0
	if e.maxActiveJobs > 0 {
		maxAccepted = maxAcceptedJobs(maxEncodes)
	}
	jobs := e.jobs.snapshot()
	suspended := 0
	if e.encodeProcesses != nil {
		pauseStates := e.encodeProcesses.PauseStates()
		suspended = e.encodeProcesses.Suspended()
		for i := range jobs {
			if state, ok := pauseStates[jobs[i].JobID]; ok && jobs[i].Phase == string(model.FFMPEGSNotification) {
				jobs[i].CanPause = !e.admissionHeld && !e.finishing
				jobs[i].IndividuallyPaused = state.Individual
				jobs[i].PausedByWorker = state.General
				if state.Paused {
					jobs[i].State = "paused"
				}
			}
		}
	}
	mode := e.pauseMode
	if mode == "" {
		mode = config.PauseFinish
	}
	return AdmissionStatus{
		Accepting:        !e.admissionHeld && !e.finishing && !e.settingsPending && !e.ramPaused && e.maxActiveJobs > 0 && withinSchedule,
		RAMPaused:        !e.admissionHeld && (e.ramPaused || (e.maxActiveJobs == 0 && e.pauseMode == config.PauseRAM)),
		ActiveJobs:       e.ActiveJobs(),
		ReservedJobs:     e.reservedJobs,
		MaxActiveJobs:    e.maxActiveJobs,
		MaxAcceptedJobs:  maxAccepted,
		ActiveEncodes:    activeEncodes,
		MaxEncodes:       maxEncodes,
		Paused:           e.encodePool != nil && e.encodePool.Paused(),
		WithinSchedule:   withinSchedule,
		Progress:         e.progress.snapshot(),
		Jobs:             jobs,
		PauseMode:        mode,
		SuspendedEncodes: suspended,
		CPUJobs:          cpuJobs, GPUJobs: gpuJobs, ActiveCPUEncodes: cpuActive, ActiveGPUEncodes: gpuActive,
		CPUMaxEncodes: cpuTarget, GPUMaxEncodes: gpuTarget, IndependentEncoders: independent,
		Finishing: e.finishing, Draining: e.admissionHeld,
	}
}

func (e *JobExecutor) SetMaxActiveJobs(max int) error {
	if max < 0 {
		return fmt.Errorf("maximum active jobs must be non-negative")
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	return e.applyLimitLocked(max, e.pauseMode, e.ramPaused)
}

func (e *JobExecutor) applyLimitLocked(max int, mode string, ram bool) error {
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.SetPaused((ram || (max == 0 && mode == config.PauseRAM)) && !e.admissionHeld); err != nil {
			return err
		}
	}
	e.finishing = false
	e.maxActiveJobs = max
	e.pauseMode = mode
	e.ramPaused = ram
	if e.encodePool != nil {
		if max > 0 {
			e.encodePool.SetTarget(max)
		}
		e.encodePool.SetPaused((ram || max == 0 || e.settingsPending) && !e.admissionHeld && !e.finishing)
	}
	if max == 0 && e.encodePool != nil {
		e.encodePool.pause()
	}
	return nil
}

func (e *JobExecutor) SetEncoderLimits(cpu, gpu int) error {
	settings := model.WorkerSettings{CPUJobs: &cpu, GPUJobs: &gpu}
	total, err := settings.Total()
	if err != nil {
		return err
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.encodePool == nil {
		e.encodePool = newEncodeConcurrency(e.workerConfig.EncodeParallelJobs())
		if e.workerConfig.EncodeConfig != nil {
			e.encodePool.setPrimary(config.EncoderKind(e.workerConfig.EncodeConfig.VideoCodec))
		}
	}
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.SetPaused((e.ramPaused || (total == 0 && e.pauseMode == config.PauseRAM)) && !e.admissionHeld); err != nil {
			return err
		}
	}
	e.encodePool.setLimits(cpu, gpu)
	e.maxActiveJobs = total
	e.finishing = false
	e.encodePool.SetPaused((total == 0 || e.ramPaused || e.settingsPending) && !e.admissionHeld)
	return nil
}

// SetRAMPaused controls the immediate hold without changing admission limits.
func (e *JobExecutor) SetRAMPaused(paused bool) error {
	if paused {
		if _, err := config.ResolvePauseMode(config.PauseRAM); err != nil {
			return err
		}
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.admissionHeld || e.finishing {
		return errors.New("RAM pause is unavailable while accepted work drains")
	}
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.SetPaused(paused); err != nil {
			return err
		}
	}
	e.pauseMode, e.ramPaused = config.PauseFinish, paused
	if e.encodePool != nil {
		e.encodePool.SetPaused(paused || e.maxActiveJobs == 0 || e.settingsPending)
	}
	return nil
}

// FinishAcceptedJobs is an operator hold released by the next explicit limit change.
func (e *JobExecutor) FinishAcceptedJobs() error {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.ResumeAll(); err != nil {
			return err
		}
	}
	e.finishing = true
	e.pauseMode = config.PauseFinish
	e.ramPaused = false
	e.maxActiveJobs = 0
	if e.encodePool != nil {
		e.encodePool.SetPaused(false)
	}
	return nil
}

// DrainAcceptedJobs closes requests but permits queued encodes to finish.
// Unlike operator pause it does not persist a preference or hold the encode pool.
func (e *JobExecutor) DrainAcceptedJobs() error {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.ResumeAll(); err != nil {
			return err
		}
	}
	e.admissionHeld = true
	e.ramPaused = false
	e.maxActiveJobs = 0
	if e.encodePool != nil {
		e.encodePool.SetPaused(false)
	}
	return nil
}

// UseManagedLimits identifies a supervised, fixed-class profile. Startup reads
// existing server overrides and keeps its local default if no override exists.
func (e *JobExecutor) UseManagedLimits() {
	e.managedLimits = true
}

// PersistEncoderLimits shares the server's atomic pair with local web/CLI controls.
func (e *JobExecutor) PersistEncoderLimits(cpu, gpu int) error {
	settings := model.WorkerSettings{CPUJobs: &cpu, GPUJobs: &gpu}
	if err := settings.ValidateWrite(); err != nil {
		return err
	}
	if e.client == nil {
		return errors.New("worker settings cannot be saved without a server client")
	}
	e.settingsMu.Lock()
	defer e.settingsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.client.SetWorkerSettings(ctx, settings); err != nil {
		return fmt.Errorf("persist encoder limits: %w", err)
	}
	if err := e.SetEncoderLimits(cpu, gpu); err != nil {
		return err
	}
	e.MarkWorkerSettingsResolved()
	return nil
}

func (e *JobExecutor) PersistMaxActiveJobs(max int) error { return e.persistLimit(max, false) }

// PersistAdmissionLimit always lets active encodes finish unless RAM/manual holds exist.
func (e *JobExecutor) PersistAdmissionLimit(max int) error { return e.persistLimit(max, true) }

func (e *JobExecutor) persistLimit(max int, admissionOnly bool) error {
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
	e.admissionMu.Lock()
	mode, ram := e.pauseMode, e.ramPaused
	if admissionOnly {
		ram = ram || (e.maxActiveJobs == 0 && mode == config.PauseRAM && !e.admissionHeld && !e.finishing)
		mode = config.PauseFinish
	}
	err := e.applyLimitLocked(max, mode, ram)
	e.admissionMu.Unlock()
	if err != nil {
		return err
	}
	e.MarkWorkerSettingsResolved()
	return nil
}

func (e *JobExecutor) RequireWorkerSettings() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.settingsPending = true
	if e.encodePool != nil && !e.admissionHeld {
		e.encodePool.SetPaused(true)
	}
}

// HoldAdmission prevents persisted server settings from reopening requests during
// a managed installation update. Accepted pipeline work continues normally.
func (e *JobExecutor) HoldAdmission() error {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.ResumeAll(); err != nil {
			return err
		}
	}
	e.admissionHeld = true
	e.ramPaused = false
	if e.encodePool != nil {
		e.encodePool.SetPaused(false)
	}
	return nil
}

func (e *JobExecutor) MarkWorkerSettingsResolved() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.settingsPending = false
	if e.encodePool != nil {
		e.encodePool.SetPaused((e.maxActiveJobs == 0 || e.ramPaused) && !e.admissionHeld && !e.finishing)
	}
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
	if e.admissionHeld || e.finishing || e.settingsPending || e.ramPaused || e.maxActiveJobs <= 0 || !e.workerConfig.AdmissionAllowedAt(now) {
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
	if err := e.pinAdmissionProfile(jobContext); err != nil {
		e.ReleaseJobSlot()
		return err
	}

	if err := e.publishTaskEvent(jobContext, model.JobNotification, model.StartedNotificationStatus, ""); err != nil {
		e.ReleaseJobSlot()
		if errors.Is(err, serverclient.ErrAssignmentExpired) {
			if cleanErr := jobContext.Clean(); cleanErr != nil {
				e.jobLogger(jobContext).Errorf("failed to discard expired workspace: %v", cleanErr)
			}
		} else if errors.Is(err, serverclient.ErrNotificationConflict) {
			if markErr := jobContext.MarkRejected(err.Error()); markErr != nil {
				e.jobLogger(jobContext).Errorf("failed to persist rejection: %v", markErr)
			}
		}
		return err
	}

	e.admissionMu.Lock()
	if e.reservedJobs > 0 {
		e.reservedJobs--
	}
	atomic.AddUint32(&e.activeJobs, 1)
	e.admissionMu.Unlock()
	e.stepExecutors[model.DownloadNotification].AddJob(jobContext)
	return nil
}

var errNotificationDelivery = errors.New("notification delivery failed")

func (e *JobExecutor) publishTaskEvent(j *job.Context, phase model.NotificationType, status model.NotificationStatus, message string) error {
	if phase == model.FFMPEGSNotification && message == "" && j.EncodeConfig != nil {
		if video, err := j.EncodeConfig.ResolveVideoSettings(); err == nil {
			message = fmt.Sprintf("%s \u00b7 %s", j.EncodeKind, video.Codec)
		}
	}
	ctx := e.serviceCtx
	if ctx == nil {
		ctx = context.Background()
	}
	event, err := j.NewLegacyEvent(e.workerConfig.Name, phase, status, message)
	if err != nil {
		return errors.Join(errNotificationDelivery, err)
	}
	if err := j.StageLegacyEvent(event); err != nil {
		return errors.Join(errNotificationDelivery, err)
	}
	if err := e.client.PublishTaskEventContext(ctx, &event); err != nil {
		return errors.Join(errNotificationDelivery, err)
	}
	if err := j.AcknowledgeLegacyEvent(event.EventID); err != nil {
		return errors.Join(errNotificationDelivery, err)
	}
	e.jobLogger(j).Logf("%s have been %s", phase, status)
	return nil
}

func (e *JobExecutor) handleDeliveryError(j *job.Context, err error) {
	e.jobLogger(j).Errorf("notification delivery stopped: %v", err)
	if errors.Is(err, serverclient.ErrAssignmentExpired) {
		if cleanErr := e.CleanJob(j); cleanErr != nil {
			e.jobLogger(j).Errorf("failed to discard expired workspace: %v", cleanErr)
		}
	} else if errors.Is(err, serverclient.ErrNotificationConflict) {
		if markErr := j.MarkRejected(err.Error()); markErr != nil {
			e.jobLogger(j).Errorf("failed to persist rejection: %v", markErr)
			return
		}
		_ = e.cleanJob(func() error { return nil })
		e.jobs.remove(j.JobId.String())
	}
}

func (e *JobExecutor) ActiveJobs() uint32 {
	return atomic.LoadUint32(&e.activeJobs)
}

func (e *JobExecutor) stepQueueRoutine(ctx context.Context, stepExecutor *step.Executor) {
	runStepQueue(ctx, stepExecutor, func(jobCtx *job.Context) {
		go e.processStep(ctx, stepExecutor, jobCtx)
	})
}

func (e *JobExecutor) encodeQueueRoutine(ctx context.Context, stepExecutor *step.Executor) {
	runEncoderQueue(ctx, stepExecutor.GetJobChan(), e.encodePool, func(jobCtx *job.Context, kind string) {
		if err := e.pinEncoderProfile(jobCtx, kind); err != nil {
			stepExecutor.OnError(jobCtx, model.FFMPEGSNotification, err)
			return
		}
		e.processStep(ctx, stepExecutor, jobCtx)
	})
}

func (e *JobExecutor) pinEncoderProfile(j *job.Context, kind string) error {
	if j.EncodeConfig != nil {
		video, err := j.EncodeConfig.ResolveVideoSettings()
		if err != nil {
			return fmt.Errorf("restore encoder profile: %w", err)
		}
		if config.EncoderKind(video.Codec) != kind {
			return fmt.Errorf("saved encoder profile does not match %s slot", kind)
		}
		if video.Codec == config.AV1SVT && j.EncodeConfig.VideoCRF == 0 {
			// Older snapshots stored the omitted CRF as zero while the default
			// was 25. Keep accepted work at its previous quality after upgrades.
			saved := *j.EncodeConfig
			saved.VideoCRF = 25
			j.EncodeConfig = &saved
		}
		j.EncodeKind = kind
		return nil
	}
	cpu, gpu, err := e.workerConfig.EncoderProfiles()
	if err != nil {
		return err
	}
	profile := gpu
	if kind == config.CPUEncoderKind {
		profile = cpu
	}
	copy := *profile
	video, err := profile.ResolveVideoSettings()
	if err != nil {
		return fmt.Errorf("resolve selected encoder profile: %w", err)
	}
	if video.Codec == config.AV1SVT {
		copy.VideoCRF = video.CRF
	}
	j.EncodeKind, j.EncodeConfig = kind, &copy
	return nil
}

func (e *JobExecutor) pinAdmissionProfile(j *job.Context) error {
	if e.encodePool == nil {
		return nil
	}
	if kind := e.encodePool.admissionKind(); kind != "" {
		return e.pinEncoderProfile(j, kind)
	}
	return nil
}

func (e *JobExecutor) processStep(ctx context.Context, stepExecutor *step.Executor, jobCtx *job.Context) {
	stepExecutor.OnJob(jobCtx)
	err := e.executeStepActions(ctx, stepExecutor, jobCtx)
	if err != nil {
		if errors.Is(err, errNotificationDelivery) || errors.Is(err, serverclient.ErrAssignmentExpired) {
			e.handleDeliveryError(jobCtx, err)
			return
		}
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

	if err := e.publishTaskEvent(jobContext, stepExecutor.NotificationType(), model.StartedNotificationStatus, ""); err != nil {
		close(actionsChan)
		wg.Wait()
		return err
	}
	if stepExecutor.NotificationType() != model.FFMPEGSNotification {
		if err := stepExecutor.AcquireJob(ctx); err != nil {
			close(actionsChan)
			wg.Wait()
			return errors.Join(errNotificationDelivery, err)
		}
	}
	for _, action := range actions {
		actionsChan <- action
	}

	close(actionsChan)
	wg.Wait()
	if stepExecutor.NotificationType() != model.FFMPEGSNotification {
		stepExecutor.ReleaseJob()
	}
	close(errs)
	if err := ctx.Err(); err != nil {
		return errors.Join(errNotificationDelivery, err)
	}

	var errorList []error
	for err := range errs {
		errorList = append(errorList, err)
	}
	err := errors.Join(errorList...)
	if err != nil {
		if errors.Is(err, serverclient.ErrAssignmentExpired) {
			return err
		}
		if deliveryErr := e.publishTaskEvent(jobContext, stepExecutor.NotificationType(), model.FailedNotificationStatus, err.Error()); deliveryErr != nil {
			return deliveryErr
		}
		return err
	}

	return e.publishTaskEvent(jobContext, stepExecutor.NotificationType(), model.CompletedNotificationStatus, "")
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

// SetPauseMode changes how operator zero limits affect active encodes.
func (e *JobExecutor) SetPauseMode(value string) error {
	mode, err := config.ResolvePauseMode(value)
	if err != nil {
		return err
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.encodeProcesses != nil {
		if err := e.encodeProcesses.SetPaused((e.ramPaused || (mode == config.PauseRAM && e.maxActiveJobs == 0)) && !e.admissionHeld && !e.finishing); err != nil {
			return err
		}
	}
	e.pauseMode = mode
	return nil
}

// SetJobPaused never changes admission, concurrency, or another job's pause.
func (e *JobExecutor) SetJobPaused(jobID string, paused bool) error {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return fmt.Errorf("invalid encoding job ID: %w", err)
	}
	if _, err := config.ResolvePauseMode(config.PauseRAM); err != nil {
		return err
	}
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	if e.admissionHeld || e.finishing {
		return errors.New("individual pause is unavailable while accepted work drains")
	}
	if e.encodeProcesses == nil {
		return control.ErrJobNotPausable
	}
	if err := e.encodeProcesses.SetJobPaused(id.String(), paused); err != nil {
		if errors.Is(err, command.ErrEncodeNotRunning) {
			return fmt.Errorf("%w: %s", control.ErrJobNotPausable, id)
		}
		return err
	}
	return nil
}
