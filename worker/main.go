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
	"time"
	"transcoder/cmd"
	"transcoder/server/web"
	"transcoder/update"
	"transcoder/version"
	"transcoder/worker/serverclient"
	"transcoder/worker/task"
)

type CmdLineOpts struct {
	WebConfig           web.WebServerConfig `mapstructure:"web"`
	WorkerConfig        task.Config         `mapstructure:"worker"`
	NoUpdateMode        bool                `mapstructure:"noUpdateMode"`
	NoUpdates           bool                `mapstructure:"noUpdates"`
	UpdateCheckInterval time.Duration       `mapstructure:"updateCheckInterval"`
}

var (
	ApplicationName = "transcoderd-worker"
	opts            CmdLineOpts
	showVersion     bool
)

func init() {

	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	cmd.WebFlags()
	var verbose bool
	pflag.BoolVar(&showVersion, "version", false, "Print version and exit")
	pflag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	pflag.Duration("updateCheckInterval", time.Minute*15, "Check for updates every X duration")

	pflag.String("worker.temporalPath", os.TempDir(), "Path used for temporal data")
	pflag.String("worker.name", hostname, "WorkerConfig Name used for statistics")
	defaultThreads := runtime.NumCPU()
	if defaultThreads > 16 {
		defaultThreads = 16
	}
	pflag.Int("worker.threads", defaultThreads, "WorkerConfig Threads")
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
	update.PFlags()
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
	if showVersion {
		version.LogVersion()
		os.Exit(0)
	}

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		wg.Add(1)
		shutdownHandler(sigs, cancel)
		wg.Done()
	}()

	if opts.NoUpdates {
		version.AppLogger().Warnf("Updates are disabled, %s won't check for updates", ApplicationName)
	}

	updater, err := update.NewUpdater(version.Version, ApplicationName, opts.NoUpdates, os.TempDir(), opts.UpdateCheckInterval)
	if err != nil {
		log.Panic(err)
	}

	if opts.NoUpdateMode || opts.NoUpdates {
		version.AppLogger().Info("Starting Worker")
		applicationRun(wg, ctx, updater)
	} else {
		updater.Run(wg, ctx)
	}

	wg.Wait()
	log.Info("Exit...")
}

func applicationRun(wg *sync.WaitGroup, ctx context.Context, updater *update.Updater) {
	printer := task.NewConsoleWorkerPrinter()
	serverClient := serverclient.NewServerClient(opts.WebConfig)
	encodeWorker := task.NewEncodeWorker(opts.WorkerConfig, serverClient, printer)

	encodeWorker.Run(wg, ctx)

	coordinator := task.NewServerCoordinator(serverClient, encodeWorker, updater, printer)
	coordinator.Run(wg, ctx)
}

func shutdownHandler(sigs chan os.Signal, cancel context.CancelFunc) {
	select {
	case <-sigs:
		cancel()
		log.Info("Termination Signal Detected...")
	}

	signal.Stop(sigs)
}
