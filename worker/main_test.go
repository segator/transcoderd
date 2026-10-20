package main

import (
	"context"
	"sync"
	"testing"

	"github.com/spf13/pflag"

	"transcoder/worker/config"
)

func TestAV1WorkerFlagDefaults(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"worker.ffmpegConfig.videoCodec", "av1_nvenc"},
		{"worker.ffmpegConfig.parallelJobs", "2"},
		{"worker.maxActiveJobs", "2"},
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
