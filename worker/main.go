//go:generate go run github.com/rakyll/statik -src=resources/statics
package main

import (
	"context"
	"fmt"
	"github.com/getlantern/systray"
	"github.com/rakyll/statik/fs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"transcoder/broker"
	"transcoder/cmd"
	"transcoder/helper"
	"transcoder/model"
	"transcoder/worker/queue"
	_ "transcoder/worker/statik"
	"transcoder/worker/task"
)

type CmdLineOpts struct {
	Broker broker.Config `mapstructure:"broker"`
	Worker task.Config   `mapstructure:"worker"`
}

var (
	opts CmdLineOpts
)

func init() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	cmd.BrokerFlags()
	pflag.String("worker.temporalPath", os.TempDir(), "Path used for temporal data")
	pflag.String("worker.workerName", hostname, "Worker Name used for statistics")
	pflag.Int("worker.workerThreads", runtime.NumCPU(), "Worker Threads")
	pflag.StringSlice("worker.acceptedJobs", []string{"encode"}, "type of jobs this Worker will accept, encode. pgsTosrt")
	pflag.Int("worker.workerEncodeJobs", 1, "Worker Encode Jobs in parallel")
	pflag.Int("worker.workerPGSJobs", 0, "Worker PGS Jobs in parallel")
	pflag.Int("worker.workerPriority", 3, "Only Accept Jobs of priority X( Priority 1= <30 Min, 2=<60 Min,3=<2 Hour,4=<3 Hour,5=>3 Hour,6-9 Manual High Priority tasks")
	pflag.Usage = usage

	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TR")
	err = viper.ReadInConfig()
	if err!=nil {
		switch err.(type){
		case viper.ConfigFileNotFoundError:
		default:
			log.Panic(err)
		}
	}
	pflag.Parse()

	viper.BindPFlags(pflag.CommandLine)
	err = viper.Unmarshal(&opts)
	if err != nil {
		log.Panic(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]...\n", os.Args[0])
	pflag.PrintDefaults()
	os.Exit(0)
}


func main() {
	log.SetLevel(log.DebugLevel)
	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		wg.Add(1)
		shutdownHandler(ctx, sigs, cancel)
		wg.Done()
	}()
	statikFS, err := fs.New()
	if err!=nil {
		panic(err)
	}
	//Prepare work environment
	prepareWorkerEnvironment(ctx,statikFS,&opts.Worker.AcceptedJobs)

	//BrokerClient System
	broker := queue.NewBrokerClientRabbit(opts.Broker, opts.Worker)
	broker.Run(wg, ctx)

	worker := task.NewWorkerClient(opts.Worker, broker)
	worker.Run(wg, ctx)

	InitializeSysTray(statikFS,sigs)
	wg.Wait()
}

func InitializeSysTray(statikFS http.FileSystem,signal chan os.Signal) {
	f, err :=statikFS.Open("/systray.enabled")
	if err!=nil{
		return
	}
	b, err := ioutil.ReadAll(f)
	if err!=nil {
		panic(err)
	}
	systrayN:= string(b)

	if systrayN != "1" {
		return
	}

	ico, err :=statikFS.Open("/systray.ico")
	if err!=nil{
		return
	}
	icoBytes, err := ioutil.ReadAll(ico)
	if err!=nil {
		panic(err)
	}

	go systray.Run(func () {
		systray.SetIcon(icoBytes)
		systray.SetTitle("Transcoder")
		systray.SetTooltip("Look at me, I'm a tooltip!")
		quitButton := systray.AddMenuItem("Close", "Close Application")
		go func() {
			for {
				select {
				case <-quitButton.ClickedCh:
					systray.Quit()

				}
			}
		}()
	},

		func() {
		signal<-nil
	})
}

func shutdownHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	select {
	case <-sigs:
		cancel()
		log.Info("Termination Signal Detected...")
	}

	signal.Stop(sigs)
}

func prepareWorkerEnvironment(ctx context.Context,statikFS http.FileSystem,acceptedJobs *task.AcceptedJobs) {
	log.Infof("Initializing Environment...")
	if acceptedJobs.IsAccepted(model.EncodeJobType) {
		if err:=helper.StatikFSFFProbe(statikFS);err!=nil {
			panic(err)
		}

		if err:=helper.StatikFSFFmpeg(statikFS);err!=nil {
			panic(err)
		}

		if err:=helper.StatikFSMKVExtract(statikFS);err!=nil {
			panic(err)
		}
	}
}

