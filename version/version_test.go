package version

import (
	"testing"
)

func TestVersionDefaults(t *testing.T) {
	// Test that version variables have default values
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Commit == "" {
		t.Error("Commit should not be empty")
	}
	if Date == "" {
		t.Error("Date should not be empty")
	}
}

func TestAppLogger(t *testing.T) {
	logger := AppLogger()
	if logger == nil {
		t.Fatal("AppLogger() should not return nil")
	}
}

func TestLogVersion(t *testing.T) {
	// This test just ensures LogVersion doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("LogVersion() panicked: %v", r)
		}
	}()

	LogVersion()
}

func TestVersionVariablesCanBeSet(t *testing.T) {
	// Save original values
	origVersion := Version
	origCommit := Commit
	origDate := Date

	// Modify values
	Version = "v1.0.0"
	Commit = "abc1234"
	Date = "2025-11-23T12:00:00Z"

	// Verify they were set
	if Version != "v1.0.0" {
		t.Errorf("Version = %s, want v1.0.0", Version)
	}
	if Commit != "abc1234" {
		t.Errorf("Commit = %s, want abc1234", Commit)
	}
	if Date != "2025-11-23T12:00:00Z" {
		t.Errorf("Date = %s, want 2025-11-23T12:00:00Z", Date)
	}

	// Restore original values
	Version = origVersion
	Commit = origCommit
	Date = origDate
}
