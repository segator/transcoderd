package main

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	pflag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	"math"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"transcoder/cmd"
	"transcoder/server/web"
	"transcoder/worker/serverclient"
	"transcoder/worker/task"
)

type CmdLineOpts struct {
	WebConfig    web.WebServerConfig `mapstructure:"web"`
	WorkerConfig task.Config         `mapstructure:"worker"`
}

var (
	opts                CmdLineOpts
	ApplicationFileName string
)

func init() {

	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	cmd.WebFlags()
	var verbose bool
	pflag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	pflag.Bool("worker.noUpdateMode", false, "Run as Updater")
	pflag.String("worker.temporalPath", os.TempDir(), "Path used for temporal data")
	pflag.String("worker.name", hostname, "WorkerConfig Name used for statistics")
	pflag.Int("worker.threads", runtime.NumCPU(), "WorkerConfig Threads")
	pflag.Int("worker.pgsConfig.parallelJobs", int(math.Ceil(float64(runtime.NumCPU())/4)), "WorkerConfig PGS Jobs in parallel")
	pflag.String("worker.pgsConfig.dotnetPath", "dotnet", "dotnet path")
	pflag.String("worker.pgsConfig.DLLPath", "./PgsToSrt.dll", "PGSToSrt.dll path")
	pflag.String("worker.pgsConfig.tessdataPath", "./tessdata", "tesseract data path")
	pflag.Int("worker.pgsConfig.tessVersion", 5, "tesseract data version")
	pflag.String("worker.pgsConfig.libleptName", "leptonica", "leptonica library name")
	pflag.Int("worker.pgsConfig.libleptVersion", 6, "leptonica library version")

	pflag.String("worker.ffmpegConfig.audioCodec", "libfdk_aac", "FFMPEG Audio Codec")
	pflag.Int("worker.ffmpegConfig.audioVBR", 5, "FFMPEG Audio VBR")
	pflag.String("worker.ffmpegConfig.videoCodec", "libx265", "FFMPEG Video Codec")
	pflag.String("worker.ffmpegConfig.videoPreset", "medium", "FFMPEG Video Preset")
	pflag.String("worker.ffmpegConfig.videoProfile", "main10", "FFMPEG Video Profile")
	pflag.Int("worker.ffmpegConfig.videoCRF", 21, "FFMPEG Video CRF")

	pflag.Var(&opts.WorkerConfig.StartAfter, "worker.startAfter", "Accept jobs only After HH:mm")
	pflag.Var(&opts.WorkerConfig.StopAfter, "worker.stopAfter", "Stop Accepting new Jobs after HH:mm")
	pflag.Usage = usage

	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/transcoderw/")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TR")
	err = viper.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
		default:
			log.Panic(err)
		}
	}
	pflag.Parse()
	log.SetFormatter(&log.TextFormatter{
		ForceColors:               true,
		FullTimestamp:             true,
		EnvironmentOverrideColors: true,
	})
	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	viper.BindPFlags(pflag.CommandLine)
	viperDecoder := viper.DecodeHook(func(source reflect.Type, target reflect.Type, data interface{}) (interface{}, error) {
		if source.Kind() != reflect.String {
			return data, nil
		}
		timeHourMinute := task.TimeHourMinute{}
		if target == reflect.TypeOf(timeHourMinute) {
			timeHourMinute.Set(data.(string))
			return timeHourMinute, nil
		}
		return data, nil
	})
	err = viper.Unmarshal(&opts, viperDecoder)
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
	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		wg.Add(1)
		shutdownHandler(ctx, sigs, cancel)
		wg.Done()
	}()

	//Prepare work environment
	printer := task.NewConsoleWorkerPrinter()
	serverClient := serverclient.NewServerClient(opts.WebConfig)
	encodeWorker := task.NewEncodeWorker(opts.WorkerConfig, serverClient, printer)

	encodeWorker.Run(wg, ctx)

	coordinator := task.NewServerCoordinator(serverClient, encodeWorker, printer)
	coordinator.Run(wg, ctx)

	wg.Wait()
	log.Info("Exit...")
}

func shutdownHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	select {
	case <-sigs:
		cancel()
		log.Info("Termination Signal Detected...")
	}

	signal.Stop(sigs)
}
