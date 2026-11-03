package main

import (
	"errors"
	"path/filepath"
	"testing"
	"transcoder/worker/control"
	"transcoder/worker/desktop"
)

func TestPauseModeChangeDoesNotRequireWorkerRestart(t *testing.T) {
	before := desktop.Settings{Domain: "https://server.test", Token: "secret", Name: "node", CPUJobs: 1, GPUJobs: 4, PauseMode: "finish"}
	after := before
	after.PauseMode = "ram"
	if !sameExceptLimit(before, after) {
		t.Fatal("mode-only change requires restart")
	}
	after.Name = "other"
	if sameExceptLimit(before, after) {
		t.Fatal("identity change should still require drain/restart")
	}
}

func TestDesktopPauseModeConfigurationAppliesWithoutDrain(t *testing.T) {
	dir := t.TempDir()
	active := desktop.Settings{Domain: "https://server.test", Token: "secret", Name: "node", CPUJobs: 1, GPUJobs: 2, PauseMode: "finish"}
	service := &supervisor{dir: dir, worker: filepath.Join(dir, "worker.exe"), state: desktop.Persisted{Active: &active, Paused: true}}
	settings := active
	settings.Token = ""
	settings.PauseMode = "ram"
	response := service.handle(desktop.Request{Command: "configure", Settings: &settings})
	if response.Error != "" {
		t.Fatal(response.Error)
	}
	if service.state.Pending != nil || !service.state.PendingLimit || service.state.Active.PauseMode != "ram" || !service.state.Paused || service.pendingDrain {
		t.Fatalf("mode edit caused drain/restart or lost pause: %+v", service.state)
	}
	saved, err := desktop.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Active.PauseMode != "ram" || saved.Active.Token != "secret" || !saved.Paused {
		t.Fatalf("saved mode/credentials/pause = %+v", saved)
	}
}

func TestEncodeOwnerRoutesOnlyActiveOwnedJobs(t *testing.T) {
	profiles := map[string]control.AdmissionStatus{
		"cpu": {Jobs: []control.JobState{{JobID: "cpu-job", Phase: "FFMPEG", CanPause: true}}},
		"gpu": {Jobs: []control.JobState{{JobID: "gpu-job", Phase: "FFMPEG", CanPause: true}, {JobID: "queued", Phase: "FFMPEG", State: "queued"}}},
	}
	for _, tt := range []struct{ id, want string }{{"cpu-job", "cpu"}, {"gpu-job", "gpu"}} {
		got, err := encodeOwner(profiles, tt.id)
		if err != nil || got != tt.want {
			t.Fatalf("encodeOwner(%s)=%s,%v", tt.id, got, err)
		}
	}
	for _, id := range []string{"unknown", "queued"} {
		if _, err := encodeOwner(profiles, id); !errors.Is(err, control.ErrJobNotPausable) {
			t.Fatalf("encodeOwner(%s) error=%v", id, err)
		}
	}
	profiles["gpu"] = profiles["cpu"]
	if _, err := encodeOwner(profiles, "cpu-job"); err == nil {
		t.Fatal("ambiguous ownership accepted")
	}
}

func TestMaintenanceRejectsIndividualPauseAndResume(t *testing.T) {
	service := &supervisor{maintenance: true}
	for _, command := range []string{"pause-job", "resume-job", "pause-encodes", "resume-encodes"} {
		response := service.handle(desktop.Request{Command: command, JobID: "00000000-0000-4000-8000-000000000001"})
		if response.Error == "" {
			t.Fatalf("%s accepted during maintenance", command)
		}
	}
}
