package config

import (
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

type Config struct {
	Database  *repository.SQLServerConfig `mapstructure:"database"`
	Scheduler *scheduler.SchedulerConfig  `mapstructure:"scheduler"`
}
