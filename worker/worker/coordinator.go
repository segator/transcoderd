package worker

import (
	"context"
	"errors"
	"os"
	"sync"
	"transcoder/model"
	"transcoder/update"
	"transcoder/worker/console"
	"transcoder/worker/serverclient"

	log "github.com/sirupsen/logrus"

	"time"
)

type ServerCoordinator struct {
	logger       console.LeveledLogger
	serverClient *serverclient.ServerClient
	worker       *JobExecutor
	updater      *update.Updater
}

func NewServerCoordinator(serverClient *serverclient.ServerClient, worker *JobExecutor, updater *update.Updater, logger console.LeveledLogger) *ServerCoordinator {
	coordinator := &ServerCoordinator{
		serverClient: serverClient,
		worker:       worker,
		logger:       logger,
		updater:      updater,
	}
	return coordinator
}

func (q *ServerCoordinator) Run(wg *sync.WaitGroup, ctx context.Context) {
	log.Info("starting server coordinator")
	q.start(ctx)
	log.Info("started server coordinator")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Info("stopping server coordinator")
		q.stop()
		wg.Done()
	}()
}

func (q *ServerCoordinator) start(ctx context.Context) {
	go q.heartbeatRoutine(ctx)
	go q.requestTaskRoutine(ctx)
}
func (q *ServerCoordinator) stop() {
	log.Info("waiting for jobs to cancel")
}

func (q *ServerCoordinator) heartbeatRoutine(ctx context.Context) {
	// Declare Worker Unique ServerCoordinator
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 5):
			if err := q.serverClient.PublishPingEvent(q.worker.RuntimeTelemetry()); err != nil {
				q.logger.Errorf("Error Publishing Ping Event: %v", err)
			}
		}
	}
}

func (q *ServerCoordinator) requestTaskRoutine(ctx context.Context) {
	if !q.loadPersistedWorkerSettings(ctx) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 5):
			if err := q.syncWorkerSettings(ctx); err != nil {
				q.logger.Errorf("Error synchronizing worker capacity; skipping admission: %v", err)
				continue
			}
			if q.worker.ReserveJobSlot() {
				release, requireUpdate, err := q.updater.CheckForUpdate()
				if err != nil {
					q.worker.ReleaseJobSlot()
					q.logger.Errorf("Error Checking For Update: %v", err)
					continue
				}
				if requireUpdate {
					q.worker.ReleaseJobSlot()
					if err := q.worker.SetMaxActiveJobs(0); err != nil {
						q.logger.Errorf("Error stopping job admission for update: %v", err)
						continue
					}
					q.logger.Logf("New version available %s; waiting for %d active jobs to finish before restarting", release.TagName, q.worker.ActiveJobs())
					if waitForActiveJobs(ctx, q.worker.ActiveJobs, time.Second) {
						os.Exit(update.ExitCode)
					}
					return
				}
				// The update check may have crossed an admission-window boundary.
				if !q.worker.WithinAdmissionSchedule() {
					q.worker.ReleaseJobSlot()
					continue
				}

				requestJobResponse, err := q.serverClient.RequestJob()
				if err != nil {
					q.worker.ReleaseJobSlot()
					if !errors.Is(err, serverclient.ErrNoJobAvailable) {
						q.logger.Errorf("Error Requesting Job: %v", err)
					}
					continue
				}

				if err := q.worker.ExecuteReservedJob(requestJobResponse.Id, requestJobResponse.EventID); err != nil {
					q.logger.Errorf("Error Preparing Job Execution: %v", err)
				}
			}
		}
	}
}

func (q *ServerCoordinator) loadPersistedWorkerSettings(ctx context.Context) bool {
	for {
		err := q.syncWorkerSettings(ctx)
		if err == nil {
			return true
		}
		q.logger.Errorf("Error loading persisted worker settings; job admission remains closed: %v", err)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (q *ServerCoordinator) syncWorkerSettings(ctx context.Context) error {
	q.worker.settingsMu.Lock()
	defer q.worker.settingsMu.Unlock()
	settingsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	settings, err := q.serverClient.GetWorkerSettings(settingsCtx)
	if err != nil {
		return err
	}
	// Avoid resetting an explicit startup parallelJobs override when no remote
	// change occurred. Local controls use this same mutex and persist first.
	if settings.Independent() {
		current := q.worker.AdmissionStatus()
		if !current.IndependentEncoders || current.CPUJobs != *settings.CPUJobs || current.GPUJobs != *settings.GPUJobs {
			if err := q.worker.SetEncoderLimits(*settings.CPUJobs, *settings.GPUJobs); err != nil {
				return err
			}
			q.logger.Logf("Applied server encoder limits CPU=%d GPU=%d", *settings.CPUJobs, *settings.GPUJobs)
		}
	} else if settings.MaxActiveJobs != nil {
		current := q.worker.AdmissionStatus()
		if current.MaxActiveJobs != *settings.MaxActiveJobs || (*settings.MaxActiveJobs > 0 && (current.MaxEncodes != *settings.MaxActiveJobs || current.IndependentEncoders)) {
			if err := q.worker.SetMaxActiveJobs(*settings.MaxActiveJobs); err != nil {
				return err
			}
			q.logger.Logf("Applied server worker job limit %d", *settings.MaxActiveJobs)
		}
	}
	q.worker.MarkWorkerSettingsResolved()
	return nil
}

func (e *JobExecutor) RuntimeTelemetry() *model.WorkerRuntime {
	status := e.AdmissionStatus()
	encoder, kind := "", "unknown"
	if e.workerConfig.EncodeConfig != nil {
		if video, err := e.workerConfig.EncodeConfig.ResolveVideoSettings(); err == nil {
			encoder, kind = video.Codec, "cpu"
			if encoder == "av1_nvenc" {
				kind = "gpu"
			}
		}
	}
	switch {
	case status.CPUJobs > 0 && status.GPUJobs > 0:
		kind = "hybrid"
	case status.CPUJobs > 0:
		kind = "cpu"
	case status.GPUJobs > 0:
		kind = "gpu"
	}
	runtime := &model.WorkerRuntime{Encoder: encoder, Kind: kind, Accepting: status.Accepting,
		WithinSchedule: status.WithinSchedule, ActiveJobs: status.ActiveJobs, ReservedJobs: status.ReservedJobs,
		MaxActiveJobs: status.MaxActiveJobs, ActiveEncodes: status.ActiveEncodes, MaxEncodes: status.MaxEncodes,
		CapacityVersion: 2, CPUJobs: status.CPUJobs, GPUJobs: status.GPUJobs,
		ActiveCPUEncodes: status.ActiveCPUEncodes, ActiveGPUEncodes: status.ActiveGPUEncodes,
		CPUMaxEncodes: status.CPUMaxEncodes, GPUMaxEncodes: status.GPUMaxEncodes}
	if cpu, gpu, err := e.workerConfig.EncoderProfiles(); err == nil {
		if video, err := cpu.ResolveVideoSettings(); err == nil {
			runtime.CPUEncoder = video.Codec
		}
		if video, err := gpu.ResolveVideoSettings(); err == nil {
			runtime.GPUEncoder = video.Codec
		}
	}
	return runtime
}

func waitForActiveJobs(ctx context.Context, activeJobs func() uint32, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for activeJobs() > 0 {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
	return true
}
