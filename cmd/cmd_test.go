package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestCommandLineConfigDefaults(t *testing.T) {
	cfg := &CommandLineConfig{}
	if cfg.NoUpdateMode {
		t.Error("NoUpdateMode should be false by default")
	}
	if cfg.NoUpdates {
		t.Error("NoUpdates should be false by default")
	}
	if cfg.Verbose {
		t.Error("Verbose should be false by default")
	}
	if cfg.Version {
		t.Error("Version should be false by default")
	}
}

func TestWorkerScheduleCLIOverridesYAML(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		zone    string
		windows []string
	}{
		{"YAML", nil, "Europe/Madrid", []string{"Mon-Fri@00:00-10:00", "Sat-Sun@00:00-24:00"}},
		{"CLI overrides", []string{"--worker.schedule.timeZone=UTC", "--worker.schedule.windows=*@00:00-08:00,*@22:00-24:00"}, "UTC", []string{"*@00:00-08:00", "*@22:00-24:00"}},
		{"CLI disables windows", []string{"--worker.schedule.windows="}, "Europe/Madrid", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("yaml")
			if err := v.ReadConfig(strings.NewReader(`worker:
  maxActiveJobs: 4
  schedule:
    timeZone: Europe/Madrid
    windows:
      - Mon-Fri@00:00-10:00
      - Sat-Sun@00:00-24:00
  ffmpegConfig:
    parallelJobs: 4
`)); err != nil {
				t.Fatal(err)
			}
			flags := pflag.NewFlagSet("schedule-test", pflag.ContinueOnError)
			flags.String("worker.schedule.timeZone", "Local", "")
			flags.StringSlice("worker.schedule.windows", nil, "")
			if err := flags.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			if err := v.BindPFlags(flags); err != nil {
				t.Fatal(err)
			}
			var got CommandLineConfig
			if err := v.Unmarshal(&got, decodeHook()); err != nil {
				t.Fatal(err)
			}
			if got.Worker == nil || got.Worker.Schedule == nil {
				t.Fatal("schedule was not decoded")
			}
			schedule := got.Worker.Schedule
			if schedule.TimeZone != tt.zone || strings.Join(schedule.Windows, ",") != strings.Join(tt.windows, ",") {
				t.Errorf("decoded schedule = %+v, want zone %s, windows %v", schedule, tt.zone, tt.windows)
			}
			if err := got.Worker.Validate(); err != nil {
				t.Errorf("Validate() error = %v", err)
			}
		})
	}
}
func TestCommandLineConfigWithValues(t *testing.T) {
	interval := time.Minute * 15
	cfg := &CommandLineConfig{
		NoUpdateMode:        true,
		NoUpdates:           true,
		UpdateCheckInterval: &interval,
		Verbose:             true,
		Version:             false,
	}
	if !cfg.NoUpdateMode {
		t.Error("NoUpdateMode should be true")
	}
	if !cfg.NoUpdates {
		t.Error("NoUpdates should be true")
	}
	if cfg.UpdateCheckInterval == nil {
		t.Fatal("UpdateCheckInterval should not be nil")
	}
	if *cfg.UpdateCheckInterval != time.Minute*15 {
		t.Errorf("UpdateCheckInterval = %v, want %v", *cfg.UpdateCheckInterval, time.Minute*15)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}
func TestCommandLineConfigNilPointers(t *testing.T) {
	cfg := &CommandLineConfig{}
	if cfg.UpdateCheckInterval != nil {
		t.Error("UpdateCheckInterval should be nil by default")
	}
	if cfg.Web != nil {
		t.Error("Web should be nil by default")
	}
	if cfg.Server != nil {
		t.Error("Server should be nil by default")
	}
	if cfg.Worker != nil {
		t.Error("Worker should be nil by default")
	}
}
