package worker

import (
	"context"
	"errors"
	"os"
	"sync"
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
			if err := q.serverClient.PublishPingEvent(); err != nil {
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
			if q.worker.ReserveJobSlot() {
				release, requireUpdate, err := q.updater.CheckForUpdate()
				if err != nil {
					q.worker.ReleaseJobSlot()
					q.logger.Errorf("Error Checking For Update: %v", err)
					continue
				}
				if requireUpdate {
					q.worker.ReleaseJobSlot()
					if err := q.worker.DrainAcceptedJobs(); err != nil {
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

				if err := q.worker.ExecuteReservedJob(requestJobResponse.Id, requestJobResponse.EventID, requestJobResponse.Name); err != nil {
					q.logger.Errorf("Error Preparing Job Execution: %v", err)
				}
			}
		}
	}
}

func (q *ServerCoordinator) loadPersistedWorkerSettings(ctx context.Context) bool {
	for {
		q.worker.settingsMu.Lock()
		settingsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		var settings *serverclient.WorkerSettings
		var err error
		if q.worker.managedLimits {
			// Synchronize service-owned limits without loading stale server overrides.
			err = q.serverClient.SetWorkerMaxActiveJobs(settingsCtx, q.worker.AdmissionStatus().MaxActiveJobs)
		} else {
			settings, err = q.serverClient.GetWorkerSettings(settingsCtx)
		}
		cancel()
		if err == nil {
			if settings != nil && settings.MaxActiveJobs != nil {
				if err := q.worker.SetMaxActiveJobs(*settings.MaxActiveJobs); err != nil {
					q.worker.settingsMu.Unlock()
					q.logger.Errorf("Error applying persisted worker job limit: %v", err)
					return false
				}
				q.logger.Logf("Applied persisted worker job limit %d", *settings.MaxActiveJobs)
			}
			q.worker.MarkWorkerSettingsResolved()
			q.worker.settingsMu.Unlock()
			return true
		}
		q.worker.settingsMu.Unlock()
		q.logger.Errorf("Error synchronizing worker settings; job admission remains closed: %v", err)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
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
