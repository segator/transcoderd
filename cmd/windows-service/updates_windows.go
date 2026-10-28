package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"transcoder/update/managed"
	"transcoder/version"
	"transcoder/worker/control"
	"transcoder/worker/desktop"

	"golang.org/x/sys/windows"
)

func (s *supervisor) updatePending() bool { return s.updates != nil && s.updates.Snapshot().Pending }

func (s *supervisor) initializeUpdates(ctx context.Context, owner string) error {
	client, err := managed.NewClient()
	if err != nil {
		return err
	}
	s.updates = managed.NewManager(s.dir, version.Version, client, managed.Hooks{Drain: s.drainForUpdate, Install: func(c *managed.Candidate, m *managed.Manifest, path string) error {
		s.mu.Lock()
		active, pending := s.state.Active, s.state.Pending
		s.mu.Unlock()
		for _, settings := range []*desktop.Settings{active, pending} {
			if settings != nil {
				if err := managed.ValidateWorkspace(filepath.Dir(s.worker), settings.WorkingDirectory, filepath.Join(s.dir, "updates")); err != nil {
					return err
				}
			}
		}
		return managed.PrepareInstall(s.dir, filepath.Dir(s.worker), owner, version.Version, c, m, path)
	}, Recover: managed.RecoverRunning})
	go s.updates.Run(ctx)
	return nil
}

func (s *supervisor) drainForUpdate() (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maintenance {
		return 0, 0, errors.New("manual Setup is running; automatic update is waiting")
	}
	active, reserved := 0, 0
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] == nil {
			if s.state.Active != nil {
				return 0, 0, fmt.Errorf("waiting for %s worker to recover accepted work", profile)
			}
			continue
		}
		r, err := control.Call(workerPipe(profile), control.Request{Command: "hold-admission"})
		if err != nil {
			return 0, 0, fmt.Errorf("waiting for %s worker control: %w", profile, err)
		}
		active += int(r.Status.ActiveJobs)
		reserved += r.Status.ReservedJobs
	}
	return active, reserved, nil
}

func (s *supervisor) startProfileLocked(profile desktop.Profile, held bool) error {
	if err := desktop.WorkerConfigForProfile(s.dir, *s.state.Active, profile, s.state.Paused); err != nil {
		return err
	}
	cmd := exec.Command(s.worker, "--noUpdates")
	cmd.Dir = s.dir
	cmd.Env = append(os.Environ(), "TRANSCODERD_INTERNAL_CONFIG="+filepath.Join(s.dir, "config-"+profile.Key+".yaml"), "TRANSCODERD_CONTROL_PIPE="+workerPipe(profile.Key), "TRANSCODERD_MANAGED_PROFILE=1")
	if held {
		cmd.Env = append(cmd.Env, "TRANSCODERD_ADMISSION_HOLD=1")
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "worker-"+profile.Key+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = f, f
	if err = cmd.Start(); err != nil {
		f.Close()
		return err
	}
	s.children[profile.Key] = cmd
	done := make(chan error, 1)
	s.done[profile.Key] = done
	go func() { err := cmd.Wait(); _ = f.Close(); done <- err }()
	return nil
}

func recordInstallation(installDir string) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("administrator privileges are required")
	}
	state, err := desktop.Load(dataDir())
	if err != nil {
		return err
	}
	for _, settings := range []*desktop.Settings{state.Active, state.Pending} {
		if settings != nil {
			if err = managed.ValidateWorkspace(installDir, settings.WorkingDirectory, filepath.Join(dataDir(), "updates")); err != nil {
				return err
			}
		}
	}
	if saved, err := managed.ReadInstallation(dataDir()); err == nil {
		if !samePath(saved.Dir, installDir) {
			return errors.New("installation path cannot change during upgrade")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	owner, err := managed.ServiceOwner()
	if err != nil {
		owner, err = currentSID()
		if err != nil {
			return err
		}
		if owner == "S-1-5-18" {
			return errors.New("a new installation requires an interactive owner")
		}
	}
	return managed.SaveInstallation(dataDir(), managed.Installation{Dir: installDir, Owner: owner})
}

func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
