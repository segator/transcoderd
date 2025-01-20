package task

import (
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"os"
	"sync"
	"transcoder/update"
	"transcoder/worker/serverclient"

	"time"
)

type ServerCoordinator struct {
	printer      *ConsoleWorkerPrinter
	serverClient *serverclient.ServerClient
	worker       *EncodeWorker
	updater      *update.Updater
}

func NewServerCoordinator(serverClient *serverclient.ServerClient, worker *EncodeWorker, updater *update.Updater, printer *ConsoleWorkerPrinter) *ServerCoordinator {
	coordinator := &ServerCoordinator{
		serverClient: serverClient,
		worker:       worker,
		printer:      printer,
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
		case <-time.After(time.Second * 30):
			if err := q.serverClient.PublishPing(); err != nil {
				q.printer.Errorf("Errorf Publishing Ping Event: %v", err)
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
					q.printer.Errorf("Errorf Checking For Update: %v", err)
					continue
				}
				if requireUpdate {
					q.printer.Log("New version available %s,exiting ...", release.TagName)
					os.Exit(update.ExitCode)
				}

				taskJob, err := q.serverClient.RequestJob(q.worker.GetName())
				if err != nil {
					if !errors.Is(err, serverclient.NoJobAvailable) {
						q.printer.Errorf("Errorf Requesting Job: %v", err)
					}
					continue
				}

				if err := q.worker.Execute(taskJob); err != nil {
					q.printer.Errorf("Errorf Preparing Job Execution: %v", err)
				}
			}
		}
	}
}
