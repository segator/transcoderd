package task

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
	"transcoder/server/web"
	"transcoder/worker/serverclient"

	log "github.com/sirupsen/logrus"

	"time"
	"transcoder/helper"
	"transcoder/model"
)

type ServerCoordinator struct {
	webServerConfig web.WebServerConfig

	printer      *ConsoleWorkerPrinter
	serverClient *serverclient.ServerClient
	worker       *EncodeWorker
}

type JobWorker struct {
	jobID        uuid.UUID
	active       bool
	encodeWorker *EncodeWorker
}

func NewServerCoordinator(serverClient *serverclient.ServerClient, worker *EncodeWorker, printer *ConsoleWorkerPrinter) *ServerCoordinator {
	coordinator := &ServerCoordinator{
		serverClient: serverClient,
		worker:       worker,
		printer:      printer,
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
			pingEvent := model.TaskEvent{
				EventType:  model.PingEvent,
				WorkerName: Q.worker.GetName(),
				EventTime:  time.Now(),
				IP:         helper.GetPublicIP(),
			}
			Q.serverClient.PublishEvent(pingEvent)
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
				taskJob, err := Q.serverClient.RequestJob()
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
