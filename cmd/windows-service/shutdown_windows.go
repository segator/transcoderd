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
	s.cancelPendingShutdownLocked()
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
	}
	// Persist the operator's stop even if a profile becomes unreachable while
	// applying it; a later service restart must not silently reopen the queue.
	next := s.state
	next.Paused = true
	if err := desktop.Save(s.dir, next); err != nil {
		return "Could not save the stopped queue"
	}
	s.state = next
	for _, profile := range []string{"cpu", "gpu"} {
		response, err := s.callWorker(profile, control.Request{Command: "set-admission-limit", MaxJobs: 0})
		if err != nil {
			return "Could not stop worker admission: " + err.Error()
		}
		if response.Error != "" || response.Status.Accepting || response.Status.MaxActiveJobs != 0 {
			return "The worker did not confirm stopped admission"
		}
	}
	var held []string
	for _, profile := range []string{"cpu", "gpu"} {
		response, err := s.callWorker(profile, control.Request{Command: "begin-restart-barrier"})
		if err != nil || response.Error != "" || !response.Status.RestartBarrier || response.Status.SafeIdle == nil {
			for _, previous := range held {
				_, _ = s.callWorker(previous, control.Request{Command: "release-restart-barrier"})
			}
			return "A worker cannot confirm safe idle; the queue remains stopped"
		}
		held = append(held, profile)
	}
	s.shutdownPending = true
	return ""
}

func (s *supervisor) cancelPendingShutdownLocked() {
	if !s.shutdownPending {
		return
	}
	s.shutdownPending = false
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] != nil {
			_, _ = s.callWorker(profile, control.Request{Command: "release-restart-barrier"})
		}
	}
}

// Called under the supervisor lock. Missing profiles/status are never treated as idle.
func (s *supervisor) checkShutdownLocked() {
	if !s.shutdownPending {
		return
	}
	if s.maintenance || s.updatePending() || s.state.Pending != nil || s.state.PendingLimit || !s.state.Paused {
		s.cancelPendingShutdownLocked()
		return
	}
	idle := true
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] == nil {
			s.cancelPendingShutdownLocked()
			return
		}
		response, err := s.callWorker(profile, control.Request{Command: "status"})
		if err != nil || response.Error != "" || response.Status.Accepting || response.Status.MaxActiveJobs != 0 {
			s.cancelPendingShutdownLocked()
			return
		}
		status := response.Status
		if status.SafeIdle == nil || !status.RestartBarrier {
			s.cancelPendingShutdownLocked()
			return
		}
		if !*status.SafeIdle || status.ReservedJobs != 0 || status.ActiveEncodes != 0 || status.SuspendedEncodes != 0 {
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
