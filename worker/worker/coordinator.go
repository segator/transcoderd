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
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 5):
			if q.worker.AcceptJobs() {
				release, requireUpdate, err := q.updater.CheckForUpdate()
				if err != nil {
					q.logger.Errorf("Error Checking For Update: %v", err)
					continue
				}
				if requireUpdate {
					q.logger.Logf("New version available %s,exiting ...", release.TagName)
					os.Exit(update.ExitCode)
				}

				requestJobResponse, err := q.serverClient.RequestJob()
				if err != nil {
					if !errors.Is(err, serverclient.ErrNoJobAvailable) {
						q.logger.Errorf("Error Requesting Job: %v", err)
					}
					continue
				}

				if err := q.worker.ExecuteJob(requestJobResponse.Id, requestJobResponse.EventID); err != nil {
					q.logger.Errorf("Error Preparing Job Execution: %v", err)
				}
			}
		}
	}
}
