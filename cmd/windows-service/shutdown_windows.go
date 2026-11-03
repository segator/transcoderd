package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"transcoder/worker/control"
	"transcoder/worker/desktop"

	"golang.org/x/sys/windows"
)

func (s *supervisor) callWorker(profile string, request control.Request) (control.Response, error) {
	if s.workerControl != nil {
		return s.workerControl(profile, request)
	}
	return control.Call(workerPipe(profile), request)
}

func (s *supervisor) stopAfterCurrentLocked(shutdown bool) string {
	s.shutdownPending = false
	s.shutdownMessage = ""
	if !shutdown {
		return "Shutdown must be explicitly requested"
	}
	if s.state.Active == nil || s.state.Pending != nil || s.state.PendingLimit {
		return "Wait for worker configuration to finish"
	}
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] == nil {
			return "Both worker profiles must be connected"
		}
		response, err := s.callWorker(profile, control.Request{Command: "finish-accepted"})
		if err != nil {
			return "Could not finish accepted work: " + err.Error()
		}
		if response.Error != "" || response.Status.Accepting || response.Status.MaxActiveJobs != 0 {
			return "The worker did not confirm stopped admission"
		}
	}
	next := s.state
	next.Paused = true
	if err := desktop.Save(s.dir, next); err != nil {
		return "Could not save the stopped queue"
	}
	s.state = next
	s.shutdownPending = true
	return ""
}

// Called under the supervisor lock. Missing profiles/status are never treated as idle.
func (s *supervisor) checkShutdownLocked() {
	if !s.shutdownPending {
		return
	}
	if s.maintenance || s.updatePending() || s.state.Pending != nil || s.state.PendingLimit || !s.state.Paused {
		s.shutdownPending = false
		return
	}
	idle := true
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] == nil {
			s.shutdownPending = false
			return
		}
		response, err := s.callWorker(profile, control.Request{Command: "status"})
		if err != nil || response.Error != "" || response.Status.Accepting || response.Status.MaxActiveJobs != 0 {
			s.shutdownPending = false
			return
		}
		status := response.Status
		if status.ActiveJobs != 0 || status.ReservedJobs != 0 || status.ActiveEncodes != 0 || status.SuspendedEncodes != 0 {
			idle = false
		}
	}
	if !idle {
		return
	}
	s.shutdownPending = false // One attempt, including when Windows refuses it.
	powerOff := s.powerOff
	if powerOff == nil {
		powerOff = shutdownWindows
	}
	if err := powerOff(); err != nil {
		s.shutdownMessage = "Could not shut down Windows: " + err.Error()
	} else {
		s.shutdownMessage = "Windows shutdown requested"
	}
}

func shutdownWindows() error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// /t 0 avoids the implicit force-close that Windows applies with a nonzero timeout.
	output, err := exec.CommandContext(ctx, filepath.Join(system, "shutdown.exe"), "/s", "/t", "0").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}
