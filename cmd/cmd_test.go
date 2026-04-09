package cmd

import (
	"testing"
	"time"
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
