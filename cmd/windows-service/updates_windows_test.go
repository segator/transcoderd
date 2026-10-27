package main

import (
	"testing"

	"transcoder/update/managed"
	"transcoder/worker/desktop"

	"golang.org/x/sys/windows"
)

func TestPendingUpdateRejectsDirectConfigurationRequests(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("protected service state needs an elevated isolated Windows runner")
	}
	dir := t.TempDir()
	if err := managed.Save(dir, managed.State{Status: managed.Status{Pending: true, Phase: "downloading", TargetVersion: "v99.1.0"}}); err != nil {
		t.Fatal(err)
	}
	s := &supervisor{dir: dir, state: desktop.Persisted{Active: &desktop.Settings{CPUJobs: 1, GPUJobs: 4}}}
	s.updates = managed.NewManager(dir, "v2.8.2", nil, managed.Hooks{})
	for _, req := range []desktop.Request{{Command: "configure", Settings: &desktop.Settings{CPUJobs: 8}}, {Command: "set-max-jobs", MaxJobs: 8}} {
		r := s.handle(req)
		if r.Error != managed.LockedMessage || r.Status.Update == nil || !r.Status.Update.Pending {
			t.Fatalf("pending update response = %+v", r)
		}
	}
	if s.state.Active.CPUJobs != 1 || s.state.Active.GPUJobs != 4 {
		t.Fatal("pending update allowed a configuration mutation")
	}
}
