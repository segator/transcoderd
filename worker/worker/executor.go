package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	activeJobs              uint32
	reservedJobs            int
	maxActiveJobs           int
	settingsPending         bool
	admissionHeld           bool
	drainEncodes            bool
	finishing               bool
	managedLimits           bool
	recoveryError           string
	protectedInventoryError string
	restartBarrier          bool
	safeIdleEnabled         bool

	workerConfig             *config.Config
	tempPath                 string
	serviceCtx               context.Context
	wg                       sync.WaitGroup
	protectedWG              sync.WaitGroup
	pipelineWG               sync.WaitGroup
	protectedOpsMu           sync.Mutex
	protectedOps             sync.WaitGroup
	protectedClosing         bool
	mu                       sync.RWMutex
	admissionMu              sync.Mutex
	settingsMu               sync.Mutex
	protectedMu              sync.Mutex
	progress                 progressRegistry
	jobs                     jobRegistry
	client                   *serverclient.ServerClient
	protectedClient          *serverclient.ProtectedClient
	protectedJobs            map[uuid.UUID]*protectedExecution
	reclaimedJobs            map[uuid.UUID]bool // guarded by admissionMu until FFmpeg starts
	initialRecoveries        []*job.Context     // populated during startup before coordinator starts
	terminalRecoveries       []*job.Context     // confirmed or uncertain completion checkpoints
	parkedRecoveries         []*job.Context     // pre-encode checkpoints awaiting server reconciliation
	protectedStarted         bool
	protectedReconciled      bool
	protectedRecoveryPending bool
	protectedWorkspace       *job.WorkspaceLock
	protectedWorkspaceID     uuid.UUID
	encodePool               *encodeConcurrency
	encodeProcesses          *command.PauseController
	pauseMode                string
	ramPaused                bool
	resources                resourceSampler

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
		if jobContext.Assignment != nil {
			jobExecutor.finishProtectedFailure(jobContext, notificationType, err)
			return
		}
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
		step.WithProtectedTransfers(jobExecutor.protectedClient),
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
		step.WithProtectedTransfers(jobExecutor.protectedClient),
		step.WithOnCompleteOpt(func(jobContext *job.Context) {
			if jobContext.Assignment != nil {
				jobExecutor.finishProtectedSuccess(jobContext)
				return
			}
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
	// Startup recovery can wait for the server. It must also stop promptly when
	// the process is asked to shut down before Run has finished starting.
	serviceCtx, cancelServiceCtx := context.WithCancel(ctx)
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
	e.serviceCtx = ctx
	e.protectedMu.Lock()
	e.protectedStarted = true
	e.protectedMu.Unlock()
	if e.resources != nil {
		e.resources.Start(ctx)
	}
	// Restoring more checkpoints than the channel capacity must not block
	// startup before the encode queue has a consumer. Recovered protected jobs
	// already have per-job holds, and production also holds reconciliation.
	protectedEncodeQueue := e.protectedClient != nil
	if protectedEncodeQueue {
		if executor := e.stepExecutors[model.FFMPEGSNotification]; executor != nil {
			e.pipelineWG.Add(1)
			go func() { defer e.pipelineWG.Done(); e.encodeQueueRoutine(ctx, executor) }()
		}
	}
	e.resumeJobsContext(ctx)
	for notificationType, stepExecutor := range e.stepExecutors {
		if protectedEncodeQueue && notificationType == model.FFMPEGSNotification {
			continue
		}
		e.pipelineWG.Add(1)
		if notificationType == model.FFMPEGSNotification {
			go func(executor *step.Executor) { defer e.pipelineWG.Done(); e.encodeQueueRoutine(ctx, executor) }(stepExecutor)
		} else {
			go func(executor *step.Executor) { defer e.pipelineWG.Done(); e.stepQueueRoutine(ctx, executor) }(stepExecutor)
		}
	}
	if e.protectedClient != nil {
		e.protectedWG.Add(1)
		go func() {
			defer e.protectedWG.Done()
			e.protectedParkingRoutine(ctx)
		}()
	}
}

func (e *JobExecutor) stop() {
	e.protectedOpsMu.Lock()
	e.protectedClosing = true
	e.protectedOpsMu.Unlock()
	e.protectedOps.Wait()
	e.protectedWG.Wait()
	if e.protectedWorkspace != nil {
		e.pipelineWG.Wait()
	}
	if e.resources != nil {
		e.resources.Stop()
	}
	for _, stepExecutor := range e.stepExecutors {
		stepExecutor.Stop()
	}
	if e.protectedWorkspace != nil {
		if err := e.protectedWorkspace.Close(); err != nil {
			log.WithError(err).Error("Failed to release protected workspace lock")
		}
	}
}

func (e *JobExecutor) beginProtectedOperation() bool {
	e.protectedOpsMu.Lock()
	defer e.protectedOpsMu.Unlock()
	if e.protectedClosing {
		return false
	}
	e.protectedOps.Add(1)
	return true
}

func (e *JobExecutor) resumeJobs() {
	e.resumeJobsContext(context.Background())
}

func (e *JobExecutor) resumeJobsContext(ctx context.Context) {
	var pendingRequest uuid.UUID
	if e.protectedWorkspace != nil {
		var pending bool
		var err error
		pendingRequest, pending, err = e.protectedWorkspace.ExistingPendingRequest()
		if err != nil {
			e.recordRecoveryError(fmt.Errorf("read pending protected request: %w", err), true)
			return
		}
		if !pending {
			pendingRequest = uuid.Nil
		}
	}
	err := filepath.Walk(e.tempPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".json" {
			// Only job snapshots use UUID filenames; other JSON files in a
			// workspace are not evidence of an unresolvable assignment.
			fileJobID, err := uuid.Parse(strings.TrimSuffix(filepath.Base(path), ".json"))
			if err != nil {
				return nil
			}
			jobContext, err := job.ReadContextFromDiskByPath(path)
			if err != nil {
				e.recordRecoveryError(fmt.Errorf("unreadable persisted job context %s: %w", path, err), true)
				return nil
			}
			if jobContext.JobId == uuid.Nil || jobContext.JobId != fileJobID {
				e.recordRecoveryError(fmt.Errorf("persisted job context %s has a mismatched job identity", path), true)
				return nil
			}
			if jobContext.Assignment != nil || jobContext.PendingProtectedEvent() != nil {
				workspace, workspaceErr := filepath.Abs(jobContext.WorkingDir)
				contextDir, contextErr := filepath.Abs(filepath.Dir(path))
				expectedDir, expectedErr := filepath.Abs(filepath.Join(e.tempPath, jobContext.JobId.String()))
				if workspaceErr != nil || contextErr != nil || expectedErr != nil || workspace != contextDir || contextDir != expectedDir {
					e.recordRecoveryError(fmt.Errorf("protected job %s has an invalid workspace", jobContext.JobId), true)
					return nil
				}
				// The request journal owns this initial checkpoint until its
				// response is reconciled. Starting it here as well would create
				// duplicate local executions after a crash at that boundary.
				if pendingRequest != uuid.Nil && jobContext.RequestID == pendingRequest && jobContext.Assignment != nil && jobContext.NextPhase == "" && jobContext.LastEvent == nil && jobContext.PendingProtectedEvent() == nil && !jobContext.Parked && !jobContext.ParkIntent {
					return nil
				}
				if e.protectedWorkspace != nil && recoverableInitialAssignment(jobContext) {
					e.initialRecoveries = append(e.initialRecoveries, jobContext)
					e.admissionMu.Lock()
					e.reservedJobs++
					e.admissionMu.Unlock()
					return nil
				}
				if e.protectedWorkspace != nil && recoverableTerminalAssignment(jobContext) {
					e.terminalRecoveries = append(e.terminalRecoveries, jobContext)
					e.admissionMu.Lock()
					e.reservedJobs++
					e.admissionMu.Unlock()
					return nil
				}
				if e.protectedWorkspace != nil && recoverablePreEncodeCheckpoint(jobContext) {
					e.parkedRecoveries = append(e.parkedRecoveries, jobContext)
					e.admissionMu.Lock()
					e.reservedJobs++
					e.admissionMu.Unlock()
					return nil
				}
				if err := e.restoreProtectedParked(ctx, jobContext); err != nil {
					e.recordRecoveryError(fmt.Errorf("protected job %s requires assignment reconciliation: %w", jobContext.JobId, err), true)
				}
				return nil
			}
			if e.protectedWorkspace != nil {
				e.recordRecoveryError(fmt.Errorf("legacy job %s requires reconciliation before protected operation", jobContext.JobId), true)
				return nil
			}
			if jobContext.LastEvent == nil {
				e.recordRecoveryError(fmt.Errorf("persisted job context %s has no last event", path), true)
				return nil
			}

			notificationType := jobContext.LastEvent.NotificationType
			if jobContext.NextPhase != "" && notificationType != model.JobNotification {
				notificationType = jobContext.NextPhase
				if notificationType == model.FFMPEGSNotification {
					workspace, workspaceErr := filepath.Abs(jobContext.WorkingDir)
					contextDir, contextErr := filepath.Abs(filepath.Dir(path))
					if workspaceErr != nil || contextErr != nil || workspace != contextDir {
						e.recordRecoveryError(fmt.Errorf("prefetched job %s has an invalid workspace", jobContext.JobId), true)
						return nil
					}
					if err := jobContext.ValidatePrefetchedSource(); err != nil {
						e.recordRecoveryError(fmt.Errorf("prefetched job %s: %w", jobContext.JobId, err), true)
						return nil
					}
				}
			}
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
					e.recordRecoveryError(fmt.Errorf("persisted job %s has unsupported status %s", jobContext.JobId, jobContext.LastEvent.Status), true)
					return nil
				}
			}

			stepExecutor, ok := e.stepExecutors[notificationType]
			if !ok || stepExecutor == nil {
				e.recordRecoveryError(fmt.Errorf("persisted job %s has unsupported phase %s", jobContext.JobId, notificationType), true)
				return nil
			}

			e.admissionMu.Lock()
			atomic.AddUint32(&e.activeJobs, 1)
			e.jobs.set(jobContext, notificationType, "queued")
			e.admissionMu.Unlock()
			stepExecutor.AddJob(jobContext)
		}

		return nil
	})

	if err != nil {
		e.recordRecoveryError(fmt.Errorf("scan worker workspace: %w", err), true)
	}
}

func (e *JobExecutor) recordRecoveryError(err error, countJob bool) {
	log.WithError(err).Error("Worker recovery is incomplete; admission remains closed")
	e.admissionMu.Lock()
	if e.recoveryError == "" {
		e.recoveryError = err.Error()
	}
	if countJob {
		atomic.AddUint32(&e.activeJobs, 1)
	}
	e.admissionMu.Unlock()
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
	runningJobs, parkedJobs, pendingJobs := 0, 0, 0
	reclaiming := len(e.reclaimedJobs) != 0
	for _, j := range jobs {
		switch j.State {
		case "running", "paused":
			runningJobs++
		case "parked":
			parkedJobs++
			reclaiming = true
		default:
			pendingJobs++
			if j.State == "reconciling" {
				reclaiming = true
			}
		}
	}
	activeJobs := e.ActiveJobs()
	if uint64(activeJobs) > uint64(len(jobs)) {
		pendingJobs += int(uint64(activeJobs) - uint64(len(jobs)))
	}
	var safeIdle *bool
	workspaceInvalid := false
	if e.safeIdleEnabled {
		workspaceInvalid = e.protectedWorkspace == nil || e.protectedWorkspace.VerifyPersistentIdentity(e.protectedWorkspaceID) != nil
		safe := !workspaceInvalid && e.protectedReconciled && e.protectedInventoryError == "" && e.restartBarrier && !e.settingsPending && e.recoveryError == "" && e.reservedJobs == 0 && activeEncodes == 0 && suspended == 0 && uint64(len(jobs)) == uint64(activeJobs) && parkedJobs == len(jobs)
		safeIdle = &safe
	}
	recoveryError := e.recoveryError
	if recoveryError == "" {
		recoveryError = e.protectedInventoryError
	}
	if recoveryError == "" && workspaceInvalid {
		recoveryError = "persistent workspace identity is unavailable"
	}
	return AdmissionStatus{
		Accepting:        !workspaceInvalid && !e.admissionHeld && !e.finishing && !e.restartBarrier && !e.settingsPending && e.recoveryError == "" && (e.protectedClient == nil || (e.protectedReconciled && !reclaiming)) && !e.ramPaused && e.maxActiveJobs > 0 && withinSchedule,
		RecoveryError:    recoveryError,
		RAMPaused:        e.ramPaused || (e.maxActiveJobs == 0 && e.pauseMode == config.PauseRAM && !e.drainEncodes),
		ActiveJobs:       activeJobs,
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
		Finishing: e.finishing, Draining: e.admissionHeld, RestartBarrier: e.restartBarrier, SafeIdle: safeIdle,
		RunningJobs: runningJobs, ParkedJobs: parkedJobs, PendingJobs: pendingJobs,
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
		if err := e.encodeProcesses.SetPaused((ram || (max == 0 && mode == config.PauseRAM)) && !e.drainEncodes); err != nil {
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
		e.encodePool.SetPaused((ram || max == 0 || e.settingsPending) && !e.drainEncodes && !e.finishing)
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
		if err := e.encodeProcesses.SetPaused((e.ramPaused || (total == 0 && e.pauseMode == config.PauseRAM)) && !e.drainEncodes); err != nil {
			return err
		}
	}
	e.encodePool.setLimits(cpu, gpu)
	e.maxActiveJobs = total
	e.finishing = false
	e.encodePool.SetPaused((total == 0 || e.ramPaused || e.settingsPending) && !e.drainEncodes)
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
	e.drainEncodes = true
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
	if e.encodePool != nil {
		e.encodePool.SetPaused(true)
	}
}

// HoldAdmission prevents new requests during a managed update without changing
// existing queue, RAM, or individual encode holds.
func (e *JobExecutor) HoldAdmission() error {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.admissionHeld = true
	e.drainEncodes = false
	if e.encodePool != nil {
		e.encodePool.SetPaused(e.ramPaused || e.maxActiveJobs == 0 || e.settingsPending)
	}
	return nil
}

// ReleaseAdmission releases only the temporary admission hold. Persisted queue,
// RAM, per-job and restart-barrier holds remain in force.
func (e *JobExecutor) ReleaseAdmission() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.admissionHeld = false
	if e.encodePool != nil {
		e.encodePool.SetPaused((e.ramPaused || e.maxActiveJobs == 0 || e.settingsPending) && !e.drainEncodes && !e.finishing)
	}
}

func (e *JobExecutor) MarkWorkerSettingsResolved() {
	e.admissionMu.Lock()
	defer e.admissionMu.Unlock()
	e.settingsPending = false
	if e.encodePool != nil {
		e.encodePool.SetPaused((e.maxActiveJobs == 0 || e.ramPaused) && !e.drainEncodes && !e.finishing)
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
	if e.admissionHeld || e.finishing || e.restartBarrier || e.settingsPending || e.recoveryError != "" || (e.protectedClient != nil && !e.protectedReconciled) || e.ramPaused || e.maxActiveJobs <= 0 || !e.workerConfig.AdmissionAllowedAt(now) {
		return false
	}
	// Reclaim every parked assignment before requesting additional work. The
	// scanner updates these states under admissionMu when ownership changes.
	if e.protectedClient != nil {
		if e.protectedWorkspace != nil && e.protectedWorkspace.VerifyPersistentIdentity(e.protectedWorkspaceID) != nil {
			return false
		}
		if len(e.reclaimedJobs) != 0 {
			return false
		}
		for _, current := range e.jobs.snapshot() {
			if current.State == "parked" || current.State == "reconciling" {
				return false
			}
		}
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

	e.publishTaskEvent(jobContext, model.JobNotification, model.StartedNotificationStatus, "")

	e.admissionMu.Lock()
	if e.reservedJobs > 0 {
		e.reservedJobs--
	}
	atomic.AddUint32(&e.activeJobs, 1)
	e.jobs.set(jobContext, model.DownloadNotification, "queued")
	e.admissionMu.Unlock()
	e.stepExecutors[model.DownloadNotification].AddJob(jobContext)
	return nil
}

func (e *JobExecutor) publishTaskEvent(jobContext *job.Context, notificationType model.NotificationType, status model.NotificationStatus, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := e.jobLogger(jobContext)
	if notificationType == model.FFMPEGSNotification && message == "" && jobContext.EncodeConfig != nil {
		if video, err := jobContext.EncodeConfig.ResolveVideoSettings(); err == nil {
			message = fmt.Sprintf("%s · %s", jobContext.EncodeKind, video.Codec)
		}
	}
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
	if jobCtx.Assignment != nil {
		e.admissionMu.Lock()
		recoveryBlocked := e.recoveryError != ""
		e.admissionMu.Unlock()
		if recoveryBlocked {
			e.blockProtectedJob(jobCtx, errors.New("worker recovery is unresolved"))
			return
		}
		execution := e.protectedExecution(jobCtx.JobId)
		if execution == nil {
			e.blockProtectedJob(jobCtx, ErrProtectedLeaseLost)
			return
		}
		guard := execution.currentGuard()
		if guard == nil || guard.ctx.Err() != nil {
			e.blockProtectedJob(jobCtx, ErrProtectedLeaseLost)
			return
		}
		ctx = guard.ctx
	}
	stepExecutor.OnJob(jobCtx)
	err := e.executeStepActions(ctx, stepExecutor, jobCtx)
	if err != nil {
		if jobCtx.Assignment != nil && (ctx.Err() != nil || errors.Is(err, ErrProtectedEventUncertain) || errors.Is(err, ErrProtectedLeaseLost)) {
			e.blockProtectedJob(jobCtx, err)
			return
		}
		stepExecutor.OnError(jobCtx, stepExecutor.NotificationType(), err)
		return
	}
	if jobCtx.Assignment != nil && ctx.Err() != nil {
		e.blockProtectedJob(jobCtx, context.Cause(ctx))
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
	if err := e.publishStepEvent(ctx, jobContext, stepExecutor.NotificationType(), model.StartedNotificationStatus, ""); err != nil {
		return err
	}
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
	if jobContext.Assignment != nil && ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err != nil {
		if jobContext.Assignment != nil && isDefinitiveProtectedFailure(err) {
			if persistErr := jobContext.RecordPhaseFailure(stepExecutor.NotificationType(), err.Error()); persistErr != nil {
				return fmt.Errorf("%w: %w", ErrProtectedEventUncertain, persistErr)
			}
		}
		if publishErr := e.publishStepEvent(ctx, jobContext, stepExecutor.NotificationType(), model.FailedNotificationStatus, err.Error()); publishErr != nil {
			return publishErr
		}
		return err
	}

	return e.publishStepEvent(ctx, jobContext, stepExecutor.NotificationType(), model.CompletedNotificationStatus, "")
}

func (e *JobExecutor) CleanJob(jobContext *job.Context) error {
	return e.cleanTrackedJob(jobContext.Clean, jobContext.JobId.String())
}

func (e *JobExecutor) cleanJob(cleanup func() error) error {
	return e.cleanTrackedJob(cleanup, "")
}

func (e *JobExecutor) cleanTrackedJob(cleanup func() error, id string) error {
	err := cleanup()
	e.admissionMu.Lock()
	atomic.AddUint32(&e.activeJobs, ^uint32(0))
	if id != "" {
		e.jobs.remove(id)
	}
	e.admissionMu.Unlock()
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
	if jobContext.Assignment != nil {
		claim := *jobContext.Assignment
		tracker.protectedClaim = &claim
		tracker.protectedClient = e.protectedClient
		tracker.protectedProgressID = uuid.NewString()
		if execution := e.protectedExecution(jobContext.JobId); execution != nil {
			if guard := execution.currentGuard(); guard != nil {
				tracker.protectedContext = guard.ctx
			}
		}
	}
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
	if e.admissionHeld {
		return errors.New("pause mode is unavailable while admission is held")
	}
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
