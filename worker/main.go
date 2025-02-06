package main

import (
	"context"
	log "github.com/sirupsen/logrus"
	pflag "github.com/spf13/pflag"
	"math"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"transcoder/cmd"
	"transcoder/update"
	"transcoder/version"
	"transcoder/worker/console"
	"transcoder/worker/serverclient"
	"transcoder/worker/worker"
)

var (
	ApplicationName = "transcoderd-worker"
	opts            cmd.CommandLineConfig
)

func init() {

	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	cmd.CommonFlags()
	pflag.String("worker.temporalPath", os.TempDir(), "Path used for temporal data")
	pflag.String("worker.name", hostname, "Worker Name used for statistics")
	defaultThreads := runtime.NumCPU()
	if defaultThreads > 16 {
		defaultThreads = 16
	}
	pflag.Int("worker.threads", defaultThreads, "Worker Threads")
	pflag.Int("worker.pgsConfig.parallelJobs", int(math.Ceil(float64(runtime.NumCPU())/4)), "Worker PGS Jobs in parallel")
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
	pflag.String("worker.ffmpegConfig.extraArgs", "", "FFMPEG Extra Args")
	pflag.Int("worker.verifyDeltaTime", 60, "FFMPEG Verify Delta Time in seconds, is the max range of time that the video can be different from the original, if is superior then the video is marked as invalid")
	pflag.Int("worker.ffmpegConfig.videoCRF", 21, "FFMPEG Video CRF")
	pflag.Duration("worker.startAfter", 0, "Accept jobs only After HH:mm")
	pflag.Duration("worker.stopAfter", 0, "Stop Accepting new Jobs after HH:mm")

	cmd.ViperConfig(&opts)
}

func main() {
	opts.PrintVersion()

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	wg.Add(1)
	go func() {
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
		err = applicationRun(wg, ctx, updater)
		if err != nil {
			log.Panic(err)
		}
	} else {
		updater.Run(wg, ctx)
	}

	wg.Wait()
	log.Info("Exit...")
}

func applicationRun(wg *sync.WaitGroup, ctx context.Context, updater *update.Updater) error {
	renderService := console.NewRenderService()
	renderService.Run(wg, ctx)

	serverClient := serverclient.NewServerClient(opts.Web, opts.Worker.Name)

	if err := serverClient.PublishPingEvent(); err != nil {
		return err
	}

	encodeWorker := worker.NewEncodeWorker(opts.Worker, serverClient, renderService)
	encodeWorker.Run(wg, ctx)

	coordinator := worker.NewServerCoordinator(serverClient, encodeWorker, updater, renderService.Logger())
	coordinator.Run(wg, ctx)
	return nil
}

func shutdownHandler(sigs chan os.Signal, cancel context.CancelFunc) {
	<-sigs
	cancel()
	log.Info("Termination Signal Detected...")
	signal.Stop(sigs)
}
