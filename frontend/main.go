package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"transcoder/frontend/web"
	"transcoder/version"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var ApplicationName = "transcoderd-frontend"

type options struct {
	Frontend web.Config `mapstructure:"frontend"`
	Version  bool       `mapstructure:"version"`
}

func readOptions(args []string) (options, error) {
	var opts options
	flags := pflag.NewFlagSet(ApplicationName, pflag.ContinueOnError)
	flags.String("frontend.listen", ":8081", "Frontend HTTP listen address")
	flags.String("frontend.apiURL", "http://localhost:8080", "TranscoderD API origin")
	flags.String("frontend.publicURL", "", "External frontend origin when behind a TLS reverse proxy")
	flags.String("config", "", "Frontend YAML configuration file")
	flags.Bool("version", false, "Print version information and exit")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if len(flags.Args()) != 0 {
		return opts, fmt.Errorf("unexpected frontend arguments")
	}
	v := viper.New()
	v.SetConfigType("yaml")
	config, _ := flags.GetString("config")
	if config != "" {
		v.SetConfigFile(config)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("/etc/transcoderd/")
		if home, err := os.UserHomeDir(); err == nil {
			v.AddConfigPath(filepath.Join(home, ".transcoderd"))
		}
		v.AddConfigPath(".")
	}
	for key, name := range map[string]string{
		"frontend.listen": "TR_FRONTEND_LISTEN", "frontend.apiURL": "TR_FRONTEND_API_URL", "frontend.publicURL": "TR_FRONTEND_PUBLIC_URL",
	} {
		if err := v.BindEnv(key, name); err != nil {
			return opts, err
		}
	}
	if err := v.BindPFlags(flags); err != nil {
		return opts, err
	}
	if err := v.ReadInConfig(); err != nil {
		var missing viper.ConfigFileNotFoundError
		if config != "" || !errors.As(err, &missing) {
			return opts, fmt.Errorf("read frontend config: %w", err)
		}
		log.Warn("No frontend config file found; using flags, environment and defaults")
	}
	if err := v.Unmarshal(&opts); err != nil {
		return opts, fmt.Errorf("decode frontend config: %w", err)
	}
	if opts.Frontend.Listen == "" {
		return opts, fmt.Errorf("frontend.listen must not be empty")
	}
	return opts, nil
}

func run(ctx context.Context, args []string) error {
	opts, err := readOptions(args)
	if err != nil {
		return err
	}
	if opts.Version {
		version.LogVersion()
		return nil
	}
	handler, err := web.NewServer(opts.Frontend)
	if err != nil {
		return err
	}
	defer handler.Close()
	listener, err := net.Listen("tcp", opts.Frontend.Listen)
	if err != nil {
		return fmt.Errorf("listen for frontend: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	version.AppLogger().WithField("listen", listener.Addr()).Info("Frontend started")
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return fmt.Errorf("stop frontend: %w", err)
		}
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	cancel()
	if err != nil && !errors.Is(err, pflag.ErrHelp) {
		log.Fatal(err)
	}
}
