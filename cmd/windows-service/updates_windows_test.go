package main

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"

	"transcoder/update/managed"
	"transcoder/worker/control"
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

type updateProfileState struct {
	mu     sync.Mutex
	status control.AdmissionStatus
}

func (p *updateProfileState) AdmissionStatus() control.AdmissionStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}
func (p *updateProfileState) PersistMaxActiveJobs(int) error {
	return errors.New("update must not persist job limits")
}
func (p *updateProfileState) HoldAdmission() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.Accepting = false
}

func TestUpdateDrainsBothProfilesAndRejectsUnreachableProfile(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("machine worker pipes require an isolated runner")
	}
	cpu := &updateProfileState{status: control.AdmissionStatus{Accepting: true, ActiveJobs: 1, MaxActiveJobs: 1}}
	gpu := &updateProfileState{status: control.AdmissionStatus{Accepting: true, ActiveJobs: 4, ReservedJobs: 1, MaxActiveJobs: 4}}
	cpuPipe, err := control.Start(workerPipe("cpu"), cpu)
	if err != nil {
		t.Fatal(err)
	}
	defer cpuPipe.Close()
	gpuPipe, err := control.Start(workerPipe("gpu"), gpu)
	if err != nil {
		t.Fatal(err)
	}
	defer gpuPipe.Close()
	s := &supervisor{state: desktop.Persisted{Active: &desktop.Settings{}}, children: map[string]*exec.Cmd{"cpu": {}, "gpu": {}}}
	active, reserved, err := s.drainForUpdate()
	if err != nil || active != 5 || reserved != 1 {
		t.Fatalf("profile drain = %d,%d,%v", active, reserved, err)
	}
	if cpu.AdmissionStatus().Accepting || gpu.AdmissionStatus().Accepting || cpu.AdmissionStatus().MaxActiveJobs != 1 || gpu.AdmissionStatus().MaxActiveJobs != 4 {
		t.Fatal("update hold changed saved limits or left admission open")
	}
	_ = gpuPipe.Close()
	if _, _, err = s.drainForUpdate(); err == nil {
		t.Fatal("unreachable GPU profile was treated as idle")
	}
}
