package task

import (
	"context"
	"errors"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"sync"
	"transcoder/server/web"
	"transcoder/update"
	"transcoder/worker/serverclient"

	"time"
)

type ServerCoordinator struct {
	webServerConfig web.Config

	printer      *ConsoleWorkerPrinter
	serverClient *serverclient.ServerClient
	worker       *EncodeWorker
	updater      *update.Updater
}

type JobWorker struct {
	jobID        uuid.UUID
	active       bool
	encodeWorker *EncodeWorker
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

func (Q *ServerCoordinator) Run(wg *sync.WaitGroup, ctx context.Context) {
	log.Info("starting server coordinator")
	Q.start(ctx)
	log.Info("started server coordinator")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Info("stopping server coordinator")
		Q.stop()
		wg.Done()
	}()
}

func (Q *ServerCoordinator) start(ctx context.Context) {
	go Q.heartbeatRoutine(ctx)
	go Q.requestTaskRoutine(ctx)
}
func (Q *ServerCoordinator) stop() {
	log.Info("waiting for jobs to cancel")
}

func (Q *ServerCoordinator) connection() {

}

func (Q *ServerCoordinator) heartbeatRoutine(ctx context.Context) {
	//Declare Worker Unique ServerCoordinator

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 30):
			if err := Q.serverClient.PublishPing(); err != nil {
				Q.printer.Error("Error Publishing Ping Event: %v", err)
			}
		}
	}
}

func (Q *ServerCoordinator) requestTaskRoutine(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 5):
			if Q.worker.AcceptJobs() {
				release, requireUpdate, err := Q.updater.CheckForUpdate()
				if err != nil {
					Q.printer.Error("Error Checking For Update: %v", err)
					continue
				}
				if requireUpdate {
					Q.printer.Log("New version available %s,exiting ...", release.TagName)
					os.Exit(update.ExitCode)
				}

				taskJob, err := Q.serverClient.RequestJob(Q.worker.GetName())
				if err != nil {
					if !errors.Is(err, serverclient.NoJobAvailable) {
						Q.printer.Error("Error Requesting Job: %v", err)
					}
					continue
				}

				if err := Q.worker.Execute(taskJob); err != nil {
					Q.printer.Error("Error Preparing Job Execution: %v", err)
				}
			}
		}
	}
}
