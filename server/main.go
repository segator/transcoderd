package main

import (
	"context"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"transcoder/cmd"
	"transcoder/helper"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
	"transcoder/server/web"
	"transcoder/update"
	"transcoder/version"
)

var (
	ApplicationName = "transcoderd-server"
	opts            cmd.CommandLineConfig
)

func init() {
	cmd.CommonFlags()

	pflag.Duration("server.scheduler.scheduleTime", time.Minute*5, "Execute the scheduling loop every X seconds")
	pflag.Duration("server.scheduler.jobTimeout", time.Hour*24, "Requeue jobs that are running for more than X minutes")
	pflag.String("server.scheduler.sourcePath", "/data/current", "Download path")
	pflag.Int64("server.scheduler.minFileSize", 1e+8, "Min File Size")
	pflag.String("server.database.Driver", "postgres", "DB Driver")
	pflag.String("server.database.Host", "localhost", "DB Host")
	pflag.Int("server.database.port", 5432, "DB Port")
	pflag.String("server.database.User", "postgres", "DB User")
	pflag.String("server.database.Password", "postgres", "DB Password")
	pflag.String("server.database.Scheme", "server", "DB Scheme")

	cmd.ViperConfig(&opts)

	//Fix Paths
	serverConfig := opts.Server
	serverConfig.Scheduler.SourcePath = filepath.Clean(serverConfig.Scheduler.SourcePath)
	helper.CheckPath(serverConfig.Scheduler.SourcePath)
}

func main() {
	opts.PrintVersion()

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		shutdownHandler(ctx, sigs, cancel)
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
		version.AppLogger().Infof("Starting server")
		applicationRun(wg, ctx, updater)
	} else {
		updater.Run(wg, ctx)
	}

	wg.Wait()
	log.Info("Exit...")
}

func applicationRun(wg *sync.WaitGroup, ctx context.Context, updater *update.Updater) {
	//Repository persist
	var repo repository.Repository
	repo, err := repository.NewSQLRepository(opts.Server.Database)
	if err != nil {
		log.Panic(err)
	}
	err = repo.Initialize(ctx)
	if err != nil {
		log.Panic(err)
	}

	//Scheduler
	scheduler, err := scheduler.NewScheduler(opts.Server.Scheduler, repo)
	if err != nil {
		log.Panic(err)
	}
	scheduler.Run(wg, ctx)

	//WebConfig Server
	var webServer *web.Server
	webServer = web.NewWebServer(opts.Web, scheduler, updater)
	webServer.Run(wg, ctx)
}

func shutdownHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	select {
	case <-ctx.Done():
		log.Info("Termination Signal Detected...")
	case <-sigs:
		cancel()
		log.Info("Termination Signal Detected...")
	}

	signal.Stop(sigs)
}
