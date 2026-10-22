package main

import (
	"context"
	"math"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"transcoder/cmd"
	"transcoder/update"
	"transcoder/version"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/serverclient"
	"transcoder/worker/worker"

	log "github.com/sirupsen/logrus"
	pflag "github.com/spf13/pflag"
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
	pflag.Int("worker.maxActiveJobs", 2, "Maximum accepted jobs on this worker")
	pflag.Int("worker.pgsConfig.parallelJobs", int(math.Ceil(float64(runtime.NumCPU())/4)), "Worker PGS Jobs in parallel")
	pflag.String("worker.pgsConfig.dotnetPath", "dotnet", "dotnet path")
	pflag.String("worker.pgsConfig.DLLPath", "./PgsToSrt.dll", "PGSToSrt.dll path")
	pflag.String("worker.pgsConfig.tessdataPath", "./tessdata", "tesseract data path")
	pflag.Int("worker.pgsConfig.tessVersion", 5, "tesseract data version")
	pflag.String("worker.pgsConfig.libleptName", "leptonica", "leptonica library name")
	pflag.Int("worker.pgsConfig.libleptVersion", 6, "leptonica library version")

	pflag.String("worker.ffmpegConfig.audioCodec", "libfdk_aac", "FFMPEG Audio Codec")
	pflag.Int("worker.ffmpegConfig.audioVBR", 5, "FFMPEG Audio VBR")
	pflag.String("worker.ffmpegConfig.videoCodec", "av1_nvenc", "Video encoder: av1_nvenc, libsvtav1, or libx265")
	pflag.String("worker.ffmpegConfig.videoPreset", "", "Video preset; defaults to p7, 4, or medium by encoder")
	pflag.String("worker.ffmpegConfig.videoProfile", "", "x265 profile; defaults to main10")
	pflag.Int("worker.ffmpegConfig.videoCQ", 0, "AV1 NVENC CQ; defaults to 34")
	pflag.Int("worker.ffmpegConfig.videoQMax", 0, "AV1 NVENC maximum QP; defaults to 100")
	pflag.Int("worker.ffmpegConfig.parallelJobs", 0, "Maximum simultaneous FFmpeg encode jobs (0 inherits worker.maxActiveJobs)")
	pflag.Int("worker.ffmpegConfig.threads", 0, "FFmpeg thread count (0 lets FFmpeg decide)")
	pflag.String("worker.ffmpegConfig.extraArgs", "", "FFMPEG Extra Args")
	pflag.Int("worker.verifyDeltaTime", 60, "FFMPEG Verify Delta Time in seconds, is the max range of time that the video can be different from the original, if is superior then the video is marked as invalid")
	pflag.Int("worker.ffmpegConfig.videoCRF", 0, "SVT-AV1 or x265 CRF; defaults to 23 or 21")
	pflag.Duration("worker.startAfter", 0, "Legacy daily admission start as a duration since midnight, e.g. 8h")
	pflag.Duration("worker.stopAfter", 0, "Legacy daily admission end as a duration since midnight, e.g. 22h")
	pflag.String("worker.schedule.timeZone", "Local", "Admission timezone (IANA name, e.g. Europe/Madrid)")
	pflag.StringSlice("worker.schedule.windows", nil, "Allowed job-request windows, e.g. 22:00-10:00 or Mon-Fri@00:00-10:00")
	pflag.StringArray("schedule", nil, "Daily job-request window HH:MM-HH:MM; repeat to add windows; follows the process timezone")

	cmd.ViperConfig(&opts)
	if pflag.Lookup("schedule").Changed {
		schedules, err := pflag.CommandLine.GetStringArray("schedule")
		if err != nil {
			log.Panic(err)
		}
		applyScheduleFlag(opts.Worker, schedules)
	}
}

func applyScheduleFlag(worker *config.Config, schedules []string) {
	if worker == nil || schedules == nil {
		return
	}
	if len(schedules) == 1 && schedules[0] == "" {
		schedules = nil
	}
	if worker.Schedule == nil {
		worker.Schedule = &config.ScheduleConfig{TimeZone: "Local"}
	}
	worker.Schedule.Windows = schedules
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
	if err := opts.Worker.Validate(); err != nil {
		return err
	}
	if schedule := opts.Worker.Schedule; schedule != nil && len(schedule.Windows) > 0 {
		log.WithFields(log.Fields{
			"time_zone": schedule.TimeZone,
			"windows":   schedule.Windows,
		}).Info("Weekly job admission enabled; accepted jobs finish after windows close")
	}
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
