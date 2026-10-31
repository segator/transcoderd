package main

import (
	"errors"
	"os/exec"
	"testing"

	"transcoder/worker/control"
	"transcoder/worker/desktop"
)

func shutdownTestService(t *testing.T) (*supervisor, map[string]control.AdmissionStatus, *int) {
	t.Helper()
	states := map[string]control.AdmissionStatus{"cpu": {}, "gpu": {}}
	calls := new(int)
	s := &supervisor{dir: t.TempDir(), state: desktop.Persisted{Active: &desktop.Settings{Domain: "https://server.test", Token: "token", Name: "node", CPUJobs: 1, GPUJobs: 1}}, children: map[string]*exec.Cmd{"cpu": {}, "gpu": {}}, powerOff: func() error { *calls++; return nil }}
	s.workerControl = func(profile string, req control.Request) (control.Response, error) {
		return control.Response{Status: states[profile]}, nil
	}
	return s, states, calls
}

func TestShutdownRequiresExplicitOptInAndCompleteWork(t *testing.T) {
	s, states, calls := shutdownTestService(t)
	s.checkShutdownLocked()
	if *calls != 0 {
		t.Fatal("ordinary idle shut down")
	}
	if err := s.stopAfterCurrentLocked(true); err != "" {
		t.Fatal(err)
	}
	for _, status := range []control.AdmissionStatus{{ActiveJobs: 1}, {ReservedJobs: 1}, {ActiveEncodes: 1}, {SuspendedEncodes: 1}} {
		states["gpu"] = status
		s.checkShutdownLocked()
		if *calls != 0 || !s.shutdownPending {
			t.Fatal("unfinished work triggered shutdown")
		}
	}
	states["gpu"] = control.AdmissionStatus{}
	s.checkShutdownLocked()
	s.checkShutdownLocked()
	if *calls != 1 || s.shutdownPending {
		t.Fatalf("shutdown calls=%d pending=%t", *calls, s.shutdownPending)
	}
}

func TestShutdownCancelsOnUncertainOrOtherLifecycleState(t *testing.T) {
	for _, name := range []string{"maintenance", "configuration", "limit", "resumed", "missing", "unreachable", "accepting", "crash"} {
		t.Run(name, func(t *testing.T) {
			s, states, calls := shutdownTestService(t)
			if err := s.stopAfterCurrentLocked(true); err != "" {
				t.Fatal(err)
			}
			switch name {
			case "maintenance":
				s.maintenance = true
			case "configuration":
				s.state.Pending = &desktop.Settings{}
			case "limit":
				s.state.PendingLimit = true
			case "resumed":
				s.state.Paused = false
			case "missing":
				delete(s.children, "gpu")
			case "unreachable":
				s.workerControl = func(string, control.Request) (control.Response, error) {
					return control.Response{}, errors.New("offline")
				}
			case "accepting":
				states["gpu"] = control.AdmissionStatus{Accepting: true}
			case "crash":
				s.onChildStoppedLocked(errors.New("exit"))
			}
			s.checkShutdownLocked()
			if *calls != 0 || s.shutdownPending {
				t.Fatalf("calls=%d pending=%t", *calls, s.shutdownPending)
			}
		})
	}
}

func TestShutdownErrorIsOneShotAndNotPersisted(t *testing.T) {
	s, _, calls := shutdownTestService(t)
	if err := s.stopAfterCurrentLocked(true); err != "" {
		t.Fatal(err)
	}
	saved, err := desktop.Load(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	restored := &supervisor{state: saved}
	if restored.shutdownPending {
		t.Fatal("shutdown survived restart")
	}
	s.powerOff = func() error { *calls++; return errors.New("refused") }
	s.checkShutdownLocked()
	s.checkShutdownLocked()
	if *calls != 1 || s.shutdownMessage == "" {
		t.Fatal("failed shutdown repeated or hidden")
	}
}

func TestStopAfterCurrentDoesNotArmOnFailure(t *testing.T) {
	s, _, calls := shutdownTestService(t)
	s.workerControl = func(string, control.Request) (control.Response, error) {
		return control.Response{}, errors.New("offline")
	}
	if err := s.stopAfterCurrentLocked(true); err == "" {
		t.Fatal("failure hidden")
	}
	s.checkShutdownLocked()
	if *calls != 0 || s.shutdownPending {
		t.Fatal("failure armed shutdown")
	}
	if err := s.stopAfterCurrentLocked(false); err == "" {
		t.Fatal("implicit shutdown accepted")
	}
}
