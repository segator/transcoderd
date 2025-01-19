package cmd

import (
	"os"
	"time"
	server "transcoder/server/config"
	"transcoder/server/web"
	"transcoder/version"
	worker "transcoder/worker/config"
)

type CommandLineConfig struct {
	NoUpdateMode        bool           `mapstructure:"noUpdateMode"`
	NoUpdates           bool           `mapstructure:"noUpdates"`
	UpdateCheckInterval *time.Duration `mapstructure:"updateCheckInterval"`
	Verbose             bool           `mapstructure:"verbose"`
	Version             bool           `mapstructure:"version"`
	Web                 *web.Config    `mapstructure:"web"`
	Server              *server.Config `mapstructure:"server"`
	Worker              *worker.Config `mapstructure:"worker"`
}

func (opts *CommandLineConfig) PrintVersion() {
	if opts.Version {
		version.LogVersion()
		os.Exit(0)
	}
}
