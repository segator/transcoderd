package main

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
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

type CmdLineOpts struct {
	Database     repository.SQLServerConfig `mapstructure:"database"`
	Web          web.WebServerConfig        `mapstructure:"web"`
	Scheduler    scheduler.SchedulerConfig  `mapstructure:"scheduler"`
	NoUpdateMode bool                       `mapstructure:"noUpdateMode"`
	NoUpdates    bool                       `mapstructure:"noUpdates"`
}

var (
	ApplicationName = "transcoderd-server"
	showVersion     = false
	opts            CmdLineOpts
)

func init() {
	//Scheduler
	var verbose bool
	pflag.BoolVar(&showVersion, "version", false, "Print version and exit")
	pflag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	pflag.Duration("scheduler.scheduleTime", time.Minute*5, "Execute the scheduling loop every X seconds")
	pflag.Duration("scheduler.jobTimeout", time.Hour*24, "Requeue jobs that are running for more than X minutes")
	pflag.String("scheduler.sourcePath", "/data/current", "Download path")
	pflag.Int64("scheduler.minFileSize", 1e+8, "Min File Size")

	//WebConfig Config

	cmd.WebFlags()

	//DB Config
	pflag.String("database.Driver", "postgres", "DB Driver")
	pflag.String("database.Host", "localhost", "DB Host")
	pflag.Int("database.port", 5432, "DB Port")
	pflag.String("database.User", "postgres", "DB User")
	pflag.String("database.Password", "postgres", "DB Password")
	pflag.String("database.Scheme", "server", "DB Scheme")
	update.PFlags()
	pflag.Usage = usage

	//pflag.Parse()
	//viper.SetConfigFile("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/transcoderd/")
	viper.AddConfigPath("$HOME/.transcoderd/")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TR")
	err := viper.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			log.Warnf("No Config File Found")
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

	urlAndDurationDecoder := viper.DecodeHook(func(source reflect.Type, target reflect.Type, data interface{}) (interface{}, error) {
		if source.Kind() != reflect.String {
			return data, nil
		}
		if target == reflect.TypeOf(url.URL{}) {
			url, err := url.Parse(data.(string))
			return url, err
		} else if target == reflect.TypeOf(time.Duration(5)) {
			return time.ParseDuration(data.(string))
		}
		return data, nil

	})
	err = viper.Unmarshal(&opts, urlAndDurationDecoder)
	if err != nil {
		log.Panic(err)
	}

	//Fix Paths
	opts.Scheduler.SourcePath = filepath.Clean(opts.Scheduler.SourcePath)
	helper.CheckPath(opts.Scheduler.SourcePath)
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
		shutdownHandler(ctx, sigs, cancel)
		wg.Done()
	}()

	if opts.NoUpdates {
		version.AppLogger().Warnf("Updates are disabled, %s won't check for updates", ApplicationName)
	}

	updater, err := update.NewUpdater(version.Version, ApplicationName, opts.NoUpdates, os.TempDir())
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
	repo, err := repository.NewSQLRepository(opts.Database)
	if err != nil {
		log.Panic(err)
	}
	err = repo.Initialize(ctx)
	if err != nil {
		log.Panic(err)
	}

	//Scheduler
	scheduler, err := scheduler.NewScheduler(opts.Scheduler, repo)
	if err != nil {
		log.Panic(err)
	}
	scheduler.Run(wg, ctx)

	//WebConfig Server
	var webServer *web.WebServer
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
