package config

import (
	"testing"
)

func TestConfigStructure(t *testing.T) {
	cfg := &Config{}

	if cfg.Database != nil {
		t.Error("Database should be nil by default")
	}

	if cfg.Scheduler != nil {
		t.Error("Scheduler should be nil by default")
	}
}

func TestConfigCanBeInitialized(t *testing.T) {
	cfg := &Config{
		Database:  nil,
		Scheduler: nil,
	}

	if cfg == nil {
		t.Fatal("Config should not be nil")
	}
}
