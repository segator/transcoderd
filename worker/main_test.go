package main

import (
	"context"
	"sync"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"transcoder/worker/config"
)

func TestAV1WorkerFlagDefaults(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"worker.ffmpegConfig.videoCodec", "av1_nvenc"},
		{"worker.ffmpegConfig.parallelJobs", "0"},
		{"worker.maxActiveJobs", "2"},
		{"worker.pgsConfig.parallelJobs", "1"},
		{"worker.ffmpegConfig.videoCQ", "0"},
		{"worker.schedule.timeZone", "Local"},
		{"worker.schedule.windows", "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got := pflag.Lookup(tt.flag)
			if got == nil || got.DefValue != tt.want {
				t.Errorf("flag %s default = %v, want %s", tt.flag, got, tt.want)
			}
		})
	}
}

func TestWorkerSelectionUsesRegisteredDefaults(t *testing.T) {
	tests := []struct {
		name, codec, preset string
		jobs                int
	}{
		{"GPU", config.AV1NVENC, "p7", 4},
		{"CPU", config.AV1SVT, "4", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			if err := v.BindPFlags(pflag.CommandLine); err != nil {
				t.Fatal(err)
			}
			v.Set("worker.name", "defaults-test")
			v.Set("worker.maxActiveJobs", tt.jobs)
			v.Set("worker.ffmpegConfig.videoCodec", tt.codec)
			var parsed struct {
				Worker *config.Config `mapstructure:"worker"`
			}
			if err := v.Unmarshal(&parsed); err != nil {
				t.Fatal(err)
			}
			if err := parsed.Worker.Validate(); err != nil {
				t.Fatal(err)
			}
			settings, err := parsed.Worker.EncodeConfig.ResolveVideoSettings()
			if err != nil {
				t.Fatal(err)
			}
			if settings.Preset != tt.preset || parsed.Worker.EncodeParallelJobs() != tt.jobs {
				t.Errorf("Resolved preset/jobs = %s/%d, want %s/%d", settings.Preset, parsed.Worker.EncodeParallelJobs(), tt.preset, tt.jobs)
			}
		})
	}
}

func TestInvalidScheduleFailsBeforeServerContact(t *testing.T) {
	previous := opts.Worker
	defer func() { opts.Worker = previous }()
	opts.Worker = &config.Config{
		MaxActiveJobs: 2,
		EncodeConfig:  &config.FFMPEGConfig{ParallelJobs: 2},
		Schedule:      &config.ScheduleConfig{TimeZone: "Not/A_Zone", Windows: []string{"*@00:00-10:00"}},
	}
	if err := applicationRun(&sync.WaitGroup{}, context.Background(), nil); err == nil {
		t.Error("applicationRun() error = nil, want schedule error before server contact")
	}
}

func TestApplyScheduleFlagOverridesConfiguredWindows(t *testing.T) {
	worker := &config.Config{
		MaxActiveJobs: 2,
		EncodeConfig:  &config.FFMPEGConfig{ParallelJobs: 2},
		Schedule:      &config.ScheduleConfig{TimeZone: "Europe/Madrid", Windows: []string{"Mon-Fri@00:00-10:00"}},
	}

	applyScheduleFlag(worker, []string{"22:00-10:00", "14:00-16:00"})

	if worker.Schedule.TimeZone != "Europe/Madrid" {
		t.Errorf("Schedule.TimeZone = %q, want Europe/Madrid", worker.Schedule.TimeZone)
	}
	if len(worker.Schedule.Windows) != 2 || worker.Schedule.Windows[0] != "22:00-10:00" || worker.Schedule.Windows[1] != "14:00-16:00" {
		t.Errorf("Schedule.Windows = %v, want [22:00-10:00 14:00-16:00]", worker.Schedule.Windows)
	}
}

func TestApplyEmptyScheduleFlagClearsConfiguredWindows(t *testing.T) {
	worker := &config.Config{
		MaxActiveJobs: 2,
		EncodeConfig:  &config.FFMPEGConfig{ParallelJobs: 2},
		Schedule:      &config.ScheduleConfig{TimeZone: "Local", Windows: []string{"22:00-10:00"}},
	}

	applyScheduleFlag(worker, []string{""})

	if len(worker.Schedule.Windows) != 0 {
		t.Errorf("Schedule.Windows = %v, want no windows", worker.Schedule.Windows)
	}
}

func TestInvalidWorkerConfigurationFailsBeforeServerContact(t *testing.T) {
	previous := opts.Worker
	defer func() { opts.Worker = previous }()
	opts.Worker = &config.Config{
		MaxActiveJobs: 2,
		EncodeConfig: &config.FFMPEGConfig{
			VideoCodec:   "invalid",
			ParallelJobs: 2,
		},
	}
	if err := applicationRun(&sync.WaitGroup{}, context.Background(), nil); err == nil {
		t.Error("applicationRun() error = nil, want configuration error")
	}
}
