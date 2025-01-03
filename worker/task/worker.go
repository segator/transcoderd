package task

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"sync"
	"transcoder/worker/serverclient"
)

func NewWorkerClient(config Config, client *serverclient.ServerClient, printer *ConsoleWorkerPrinter) *WorkerRuntime {
	runtime := &WorkerRuntime{
		config:  config,
		printer: printer,
	}

	runtime.PGSWorker = NewPGSWorker(config, fmt.Sprintf("%s-%d", "pgsToSrt"))
	runtime.EncodeWorker = NewEncodeWorker(config, runtime.PGSWorker, client, fmt.Sprintf("%s-%d", "encoder", 1), printer)
	return runtime
}

type WorkerRuntime struct {
	config       Config
	EncodeWorker *EncodeWorker
	PGSWorker    *PGSWorker
	serverClient *serverclient.ServerClient
	printer      *ConsoleWorkerPrinter
}

func (W *WorkerRuntime) Run(wg *sync.WaitGroup, ctx context.Context) {
	log.Info("Starting Worker Client...")
	W.start(ctx)
	log.Info("Started Worker Client...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Info("Stopping Worker Client...")
		W.stop()
		wg.Done()
	}()
}
func (W *WorkerRuntime) start(ctx context.Context) {
	W.EncodeWorker.Start(ctx)
	log.Info("Initializing encode Worker")
}

func (W *WorkerRuntime) stop() {
	log.Warnf("Stopping all Workers")

}

func (W *WorkerRuntime) GetName() string {
	return W.config.Name
}
