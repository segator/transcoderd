//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"transcoder/model"
	"transcoder/update/managed"
	"transcoder/version"
	"transcoder/worker/control"
	"transcoder/worker/desktop"

	"github.com/Microsoft/go-winio"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "TranscoderDWorker"
const pipePrefix = `\\.\pipe\transcoderd-desktop-`

func currentSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

func dataDir() string { return filepath.Join(os.Getenv("ProgramData"), "TranscoderD") }

func protectDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	ptr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("service data directory cannot be a reparse point")
	}
	existing, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	existingOwner, _, err := existing.Owner()
	if err != nil {
		return err
	}
	sid, err := currentSID()
	if err != nil {
		return err
	}
	if owner := existingOwner.String(); owner != sid && owner != "S-1-5-18" && owner != "S-1-5-32-544" {
		return errors.New("service data directory is owned by a different user")
	}
	for _, name := range []string{"service.json", "config.yaml", "config-cpu.yaml", "config-gpu.yaml", "worker.log", "worker-cpu.log", "worker-gpu.log", "work"} {
		path, err := windows.UTF16PtrFromString(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		a, err := windows.GetFileAttributes(path)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			continue
		}
		if err != nil {
			return err
		}
		if a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("service data cannot contain reparse points")
		}
	}
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil)
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return err
	}
	return nil
}

func install(owner string) error {
	if saved, err := managed.ReadInstallation(dataDir()); err == nil {
		owner = saved.Owner
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := windows.StringToSid(owner); err != nil {
		return fmt.Errorf("invalid installing user SID: %w", err)
	}
	if err := protectDir(dataDir()); err != nil {
		return fmt.Errorf("protect service settings: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err = managed.SaveInstallation(dataDir(), managed.Installation{Dir: filepath.Dir(exe), Owner: owner}); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	if old, err := m.OpenService(serviceName); err == nil {
		old.Close()
		return errors.New("service already exists; stop and remove it before reinstalling")
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{DisplayName: "TranscoderD Worker", Description: "TranscoderD background worker", StartType: mgr.StartAutomatic}, "--run", owner)
	if err != nil {
		return err
	}
	defer s.Close()
	if err = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}}, 3600); err != nil {
		_ = s.Delete()
		return err
	}
	return s.Start()
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped {
		return errors.New("stop the service before uninstalling")
	}
	return s.Delete()
}

func prepareUpgrade(wait bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()
	state, err := s.Query()
	if err != nil {
		return err
	}
	if state.State == svc.Stopped {
		return nil
	}
	config, err := s.Config()
	if err != nil {
		return err
	}
	owner := regexp.MustCompile(`S-1-(?:[0-9]+-)*[0-9]+`).FindString(config.BinaryPathName)
	if owner == "" {
		return errors.New("cannot identify the service owner")
	}
	ctx := context.Background()
	if wait {
		var cancel context.CancelFunc
		ctx, cancel, err = installerWaitContext()
		if err != nil {
			return err
		}
		defer cancel()
		// Preserve the service's maintenance/shutdown cancellation semantics.
		// Old services return this specific busy response after holding CPU only;
		// the new helper still holds and checks BOTH profiles before proceeding.
		response, err := upgradeServiceStatus(owner, desktop.Request{Command: "prepare-upgrade"})
		if err != nil && response.Error != manualUpgradeBusyMessage {
			return err
		}
		if err := waitForManualUpgrade(ctx, func() (int, int, error) {
			response, err := upgradeServiceStatus(owner, desktop.Request{Command: "status"})
			if err != nil {
				return 0, 0, err
			}
			return holdProfilesForManualUpgrade(response.Status, func(profile string, request control.Request) (control.Response, error) {
				return control.Call(workerPipe(profile), request)
			})
		}, 2*time.Second, os.Stdout); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup ended before service replacement: %w", err)
	}
	if _, err := upgradeServiceStatus(owner, desktop.Request{Command: "prepare-upgrade"}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup ended before service replacement: %w", err)
	}
	if _, err = s.Control(svc.Stop); err != nil {
		return err
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		state, err = s.Query()
		if err != nil {
			return err
		}
		if state.State == svc.Stopped {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("service did not stop")
}

type supervisor struct {
	mu              sync.Mutex
	state           desktop.Persisted
	dir, worker     string
	children        map[string]*exec.Cmd
	done            map[string]chan error
	message         string
	pendingDrain    bool
	readyToApply    bool
	maintenance     bool
	queue           *model.QueueSnapshot
	queueError      string
	pipeSuffix      string
	systemOwner     bool
	testCancel      context.CancelFunc
	updates         *managed.Manager
	updateHeld      bool
	shutdownPending bool
	shutdownMessage string
	powerOff        func() error
	workerControl   func(string, control.Request) (control.Response, error)
}

func (s *supervisor) snapshotLocked() desktop.Status {
	status := desktop.Status{ShutdownPending: s.shutdownPending, Configured: s.state.Active != nil, Running: len(s.children) > 0, Version: version.Version, Pending: s.state.Pending != nil || s.state.PendingLimit, Message: s.message, Queue: s.queue, QueueError: s.queueError, Profiles: make(map[string]control.AdmissionStatus)}
	if s.shutdownMessage != "" {
		status.Message = s.shutdownMessage
	}
	// Only isolated fixture builds can inject a failed post-install health check.
	if managed.FixtureMode == "true" && version.Version == "v99.1.0" {
		if _, err := os.Stat(filepath.Join(s.dir, "updates", "fixture-fail-health")); err == nil {
			status.Version = "unhealthy-fixture"
		}
	}
	if s.updates != nil {
		update := s.updates.Snapshot()
		status.Update = &update
	}
	value := s.state.Active
	if s.state.Pending != nil {
		value = s.state.Pending
	}
	if value != nil {
		settings := value.Public()
		if settings.WorkingDirectory == "" {
			settings.WorkingDirectory = filepath.Join(s.dir, "work")
		}
		status.Settings = &settings
	}
	combined := control.AdmissionStatus{WithinSchedule: true, Paused: true}
	if s.state.Active != nil {
		combined.PauseMode = s.state.Active.Normalized().PauseMode
	}
	enabled := map[string]bool{}
	if s.state.Active != nil {
		for _, profile := range s.state.Active.Profiles() {
			enabled[profile.Key] = profile.Jobs > 0
		}
	}
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] == nil {
			continue
		}
		r, err := control.Call(workerPipe(profile), control.Request{Command: "status"})
		if err != nil {
			status.Message = "A worker profile is starting or not responding"
			continue
		}
		profileStatus := r.Status
		for i := range profileStatus.Jobs {
			profileStatus.Jobs[i].Profile = profile
		}
		for i := range profileStatus.Progress {
			profileStatus.Progress[i].Profile = profile
		}
		status.Profiles[profile] = profileStatus
		combined.Accepting = combined.Accepting || profileStatus.Accepting
		combined.ActiveJobs += profileStatus.ActiveJobs
		combined.ReservedJobs += profileStatus.ReservedJobs
		combined.MaxActiveJobs += profileStatus.MaxActiveJobs
		combined.MaxAcceptedJobs += profileStatus.MaxAcceptedJobs
		combined.ActiveEncodes += profileStatus.ActiveEncodes
		combined.RAMPaused = combined.RAMPaused || profileStatus.RAMPaused
		combined.SuspendedEncodes += profileStatus.SuspendedEncodes
		combined.Paused = combined.Paused && profileStatus.Paused
		if enabled[profile] {
			combined.MaxEncodes += profileStatus.MaxEncodes
		}
		combined.WithinSchedule = combined.WithinSchedule && profileStatus.WithinSchedule
		combined.Jobs = append(combined.Jobs, profileStatus.Jobs...)
		combined.Progress = append(combined.Progress, profileStatus.Progress...)
	}
	if len(status.Profiles) > 0 {
		status.Worker = &combined
	}
	return status
}

func sameExceptLimit(a, b desktop.Settings) bool {
	a = a.Normalized()
	b = b.Normalized()
	a.PauseMode, b.PauseMode = "", ""
	a.CPUJobs, a.GPUJobs = 0, 0
	b.CPUJobs, b.GPUJobs = 0, 0
	a.MaxJobs, b.MaxJobs = 0, 0
	a.Codec, b.Codec = "", ""
	return reflect.DeepEqual(a, b)
}

func workerPipe(profile string) string { return `\\.\pipe\transcoderd-worker-` + profile }

func (s *supervisor) allChildrenStoppedLocked() bool {
	return s.children["cpu"] == nil && s.children["gpu"] == nil
}

func (s *supervisor) handle(req desktop.Request) desktop.Response {
	if req.Command == "stop-after-current" && !req.ShutdownAfterCurrent {
		zero := 0
		req.Command, req.CPUJobs, req.GPUJobs = "set-max-jobs", &zero, &zero
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	result := desktop.Response{}
	if (s.maintenance || s.updatePending()) && (req.Command == "stop-after-current" || req.Command == "configure" || req.Command == "set-max-jobs" || req.Command == "pause-job" || req.Command == "resume-job" || req.Command == "pause-encodes" || req.Command == "resume-encodes") {
		result.Error = managed.LockedMessage
		result.Status = s.snapshotLocked()
		return result
	}
	if s.shutdownPending && (req.Command == "pause-job" || req.Command == "resume-job" || req.Command == "pause-encodes" || req.Command == "resume-encodes") {
		result.Error = "Resume queue to cancel shutdown before changing encoding pauses"
		result.Status = s.snapshotLocked()
		return result
	}
	switch req.Command {
	case "status":
	case "check-update":
		if s.updates == nil {
			result.Error = "The update service is unavailable"
		} else {
			s.updates.CheckNow()
		}
	case "shutdown-test":
		if s.testCancel == nil {
			result.Error = "Unknown command"
		} else {
			go func() { time.Sleep(200 * time.Millisecond); s.testCancel() }()
		}
	case "stop-after-current":
		result.Error = s.stopAfterCurrentLocked(req.ShutdownAfterCurrent)
	case "configure":
		s.shutdownPending = false
		s.shutdownMessage = ""
		if req.Settings == nil {
			result.Error = "Settings are missing"
			break
		}
		value := req.Settings.Normalized()
		value.WorkingDirectory = strings.TrimSpace(value.WorkingDirectory)
		current := s.state.Active
		if s.state.Pending != nil {
			current = s.state.Pending
		}
		if value.Token == "" && current != nil {
			value.Token = current.Token
		}
		value.Domain = strings.TrimRight(strings.TrimSpace(value.Domain), "/")
		value.Name = strings.TrimSpace(value.Name)
		if err := value.Validate(); err != nil {
			result.Error = err.Error()
			break
		}
		if err := managed.ValidateWorkspace(filepath.Dir(s.worker), value.WorkingDirectory, filepath.Join(s.dir, "updates")); err != nil {
			result.Error = err.Error()
			break
		}
		next := s.state
		switch {
		case next.Active == nil:
			next.Active = &value
			next.PendingLimit = true
		case next.Pending == nil && sameExceptLimit(*next.Active, value):
			next.Active = &value
			next.PendingLimit = true
		default:
			next.Pending = &value
			next.PendingLimit = true
		}
		if err := desktop.Save(s.dir, next); err != nil {
			result.Error = "Could not save settings"
			break
		}
		s.state = next
	case "pause-job", "resume-job":
		id, err := uuid.Parse(req.JobID)
		if err != nil {
			result.Error = "Invalid encoding job ID"
			break
		}
		if s.state.Pending != nil || s.state.PendingLimit {
			result.Error = "Wait for pending configuration to finish"
			break
		}
		profile, err := encodeOwner(s.snapshotLocked().Profiles, id.String())
		if err != nil {
			result.Error = err.Error()
			break
		}
		if _, err := control.Call(workerPipe(profile), control.Request{Command: req.Command, JobID: id.String()}); err != nil {
			result.Error = err.Error()
		}
	case "pause-encodes", "resume-encodes":
		if s.state.Active == nil || s.state.Pending != nil || s.state.PendingLimit {
			result.Error = "Wait for worker configuration to finish"
			break
		}
		for _, profile := range []string{"cpu", "gpu"} {
			if s.children[profile] == nil {
				result.Error = "The " + profile + " worker is reconnecting"
				break
			}
			if _, err := control.Call(workerPipe(profile), control.Request{Command: req.Command}); err != nil {
				result.Error = "Could not change " + profile + " RAM pause: " + err.Error()
				break
			}
		}
		if result.Error != "" {
			break
		}
		next := s.state
		next.RAMPaused = req.Command == "pause-encodes"
		if err := desktop.Save(s.dir, next); err != nil {
			result.Error = "RAM pause changed but could not be saved"
			break
		}
		s.state = next
	case "set-max-jobs":
		s.shutdownPending = false
		s.shutdownMessage = ""
		if s.state.Active == nil {
			result.Error = "Configure the server first"
			break
		}
		if s.state.Pending != nil {
			result.Error = "Settings are waiting to be applied"
			break
		}
		if s.allChildrenStoppedLocked() {
			result.Error = "The worker is not connected"
			break
		}
		cpuJobs, gpuJobs := s.state.Active.CPUJobs, s.state.Active.GPUJobs
		if req.CPUJobs != nil || req.GPUJobs != nil {
			if req.CPUJobs != nil {
				cpuJobs = *req.CPUJobs
			}
			if req.GPUJobs != nil {
				gpuJobs = *req.GPUJobs
			}
		} else {
			if req.MaxJobs < 0 || req.MaxJobs > 64 {
				result.Error = "The number of concurrent encodes must be between 0 and 64"
				break
			}
			switch {
			case req.MaxJobs == 0:
				cpuJobs, gpuJobs = 0, 0
			case cpuJobs > 0 && gpuJobs == 0:
				cpuJobs = req.MaxJobs
			case gpuJobs > 0 && cpuJobs == 0:
				gpuJobs = req.MaxJobs
			default:
				result.Error = "Use separate CPU and GPU limits"
			}
		}
		if result.Error != "" {
			break
		}
		if cpuJobs < 0 || gpuJobs < 0 || cpuJobs > 64 || gpuJobs > 64 || cpuJobs+gpuJobs > 64 {
			result.Error = "CPU and GPU concurrent encodes must each be between 0 and 64 and total no more than 64"
			break
		}
		for _, target := range []struct {
			key  string
			jobs int
		}{{"cpu", cpuJobs}, {"gpu", gpuJobs}} {
			if s.children[target.key] == nil {
				result.Error = "The " + target.key + " worker is reconnecting; try again shortly"
				break
			}
			if _, err := control.Call(workerPipe(target.key), control.Request{Command: "set-pause-mode", PauseMode: "finish"}); err != nil {
				result.Error = "Could not change the " + target.key + " worker pause mode: " + err.Error()
				break
			}
			if _, err := control.Call(workerPipe(target.key), control.Request{Command: "set-admission-limit", MaxJobs: target.jobs}); err != nil {
				result.Error = "Could not change the " + target.key + " worker limit"
				break
			}
		}
		if result.Error != "" {
			break
		}
		next := s.state
		if cpuJobs+gpuJobs == 0 {
			next.Paused = true
		} else {
			copy := *next.Active
			copy.CPUJobs, copy.GPUJobs = cpuJobs, gpuJobs
			next.Active = &copy
			next.Paused = false
		}
		next.PendingLimit = false
		if err := desktop.Save(s.dir, next); err != nil {
			result.Error = "The limit changed, but could not be saved locally"
			break
		}
		s.state = next
	case "prepare-upgrade":
		result.Error = s.prepareUpgradeLocked()
	default:
		result.Error = "Unknown command"
	}
	result.Status = s.snapshotLocked()
	return result
}

func (s *supervisor) run(ctx context.Context) {
	if s.children == nil {
		s.children = map[string]*exec.Cmd{}
	}
	if s.done == nil {
		s.done = map[string]chan error{}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		updatePending := s.updatePending()
		if updatePending || s.maintenance || s.state.Pending != nil || s.state.PendingLimit {
			s.shutdownPending = false
		}
		s.checkShutdownLocked()
		if updatePending {
			s.updateHeld = true
			if s.state.Active != nil && !s.maintenance {
				for _, profile := range s.state.Active.Profiles() {
					if s.children[profile.Key] == nil {
						if err := s.startProfileLocked(profile, true); err != nil {
							s.message = err.Error()
						}
					}
					if s.children[profile.Key] != nil {
						_, _ = control.Call(workerPipe(profile.Key), control.Request{Command: "hold-admission"})
					}
				}
			}
		} else if s.updateHeld {
			for profile := range s.children {
				_, _ = control.Call(workerPipe(profile), control.Request{Command: "shutdown"})
			}
			if s.allChildrenStoppedLocked() {
				s.updateHeld = false
			}
		}
		if !s.maintenance && !updatePending && !s.updateHeld && s.state.Active != nil {
			if s.state.Pending == nil && s.state.PendingLimit && s.allChildrenStoppedLocked() {
				ok := true
				for _, profile := range s.state.Active.Profiles() {
					if s.state.Paused {
						profile.Jobs = 0
					}
					if err := persistInitialLimit(ctx, *s.state.Active, profile); err != nil {
						ok = false
						s.message = "Could not connect to the server or save worker limits; retrying"
						break
					}
				}
				if ok {
					next := s.state
					next.PendingLimit = false
					if err := desktop.Save(s.dir, next); err != nil {
						s.message = "Could not save configuration state"
					} else {
						s.state = next
					}
				}
			}
			if s.state.Pending != nil {
				if !s.pendingDrain {
					allDrained := true
					for _, profile := range []string{"cpu", "gpu"} {
						if s.children[profile] != nil {
							_, err := control.Call(workerPipe(profile), control.Request{Command: "drain"})
							if err != nil {
								allDrained = false
							}
						}
					}
					if allDrained {
						s.pendingDrain = true
					}
				}
				if s.pendingDrain {
					allIdle := true
					for _, profile := range []string{"cpu", "gpu"} {
						if s.children[profile] != nil {
							r, err := control.Call(workerPipe(profile), control.Request{Command: "status"})
							if err != nil || r.Status.ActiveJobs != 0 || r.Status.ReservedJobs != 0 {
								allIdle = false
							}
						}
					}
					if allIdle {
						for _, profile := range []string{"cpu", "gpu"} {
							if s.children[profile] != nil {
								_, _ = control.Call(workerPipe(profile), control.Request{Command: "shutdown"})
							}
						}
						s.readyToApply = true
					}
				}
				if s.allChildrenStoppedLocked() && s.readyToApply {
					next := s.state
					next.Active = next.Pending
					next.Pending = nil
					next.PendingLimit = true
					if desktop.Save(s.dir, next) == nil {
						s.state = next
					} else {
						s.message = "Could not apply pending settings"
					}
					s.readyToApply = false
					s.pendingDrain = false
				}
			} else if s.state.PendingLimit && !s.allChildrenStoppedLocked() {
				ok := true
				for _, profile := range s.state.Active.Profiles() {
					if s.state.Paused {
						profile.Jobs = 0
					}
					if s.children[profile.Key] != nil {
						if _, err := control.Call(workerPipe(profile.Key), control.Request{Command: "set-pause-mode", PauseMode: "finish"}); err != nil {
							ok = false
							s.message = "Could not apply " + profile.Key + " pause mode: " + err.Error()
							continue
						}
						if _, err := control.Call(workerPipe(profile.Key), control.Request{Command: "set-admission-limit", MaxJobs: profile.Jobs}); err != nil {
							ok = false
						}
					}
					if s.children[profile.Key] == nil {
						if err := persistInitialLimit(ctx, *s.state.Active, profile); err != nil {
							ok = false
						}
					}
				}
				if ok {
					next := s.state
					next.PendingLimit = false
					if desktop.Save(s.dir, next) == nil {
						s.state = next
					}
				}
			}
			if s.state.Pending == nil && !s.state.PendingLimit {
				for _, profile := range s.state.Active.Profiles() {
					if s.children[profile.Key] != nil {
						continue
					}
					if err := desktop.WorkerConfigForProfile(s.dir, *s.state.Active, profile, s.state.Paused, s.state.RAMPaused); err != nil {
						s.message = "Could not prepare the worker configuration"
						break
					}
					cmd := exec.Command(s.worker, "--noUpdates")
					cmd.Dir = s.dir
					cmd.Env = append(os.Environ(), "TRANSCODERD_INTERNAL_CONFIG="+filepath.Join(s.dir, "config-"+profile.Key+".yaml"), "TRANSCODERD_CONTROL_PIPE="+workerPipe(profile.Key), "TRANSCODERD_MANAGED_PROFILE=1")
					logFile, err := os.OpenFile(filepath.Join(s.dir, "worker-"+profile.Key+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
					if err != nil {
						s.message = "Could not open a worker log"
						break
					}
					cmd.Stdout, cmd.Stderr = logFile, logFile
					if err = cmd.Start(); err != nil {
						_ = logFile.Close()
						s.message = "Could not start the " + profile.Key + " worker"
						break
					}
					s.children[profile.Key] = cmd
					ch := make(chan error, 1)
					s.done[profile.Key] = ch
					go func(c *exec.Cmd, f *os.File, done chan error) { err := c.Wait(); _ = f.Close(); done <- err }(cmd, logFile, ch)
				}
				if len(s.children) == 2 {
					s.message = ""
				}
			}
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			s.mu.Lock()
			children := make(map[string]*exec.Cmd, len(s.children))
			done := make(map[string]chan error, len(s.done))
			for profile, child := range s.children {
				children[profile] = child
				done[profile] = s.done[profile]
			}
			s.mu.Unlock()
			for profile, child := range children {
				_, _ = control.Call(workerPipe(profile), control.Request{Command: "shutdown"})
				select {
				case <-done[profile]:
				case <-time.After(8 * time.Second):
					_ = child.Process.Kill()
					<-done[profile]
				}
			}
			return
		case err := <-s.done["cpu"]:
			s.mu.Lock()
			delete(s.children, "cpu")
			delete(s.done, "cpu")
			s.onChildStoppedLocked(err)
			s.mu.Unlock()
		case err := <-s.done["gpu"]:
			s.mu.Lock()
			delete(s.children, "gpu")
			delete(s.done, "gpu")
			s.onChildStoppedLocked(err)
			s.mu.Unlock()
		case <-ticker.C:
		}
	}
}

func (s *supervisor) onChildStoppedLocked(err error) {
	s.shutdownPending = false
	if s.state.Pending != nil && s.readyToApply && s.allChildrenStoppedLocked() {
		next := s.state
		next.Active = next.Pending
		next.Pending = nil
		next.PendingLimit = true
		if desktop.Save(s.dir, next) == nil {
			s.state = next
		} else {
			s.message = "Could not apply pending settings"
		}
		s.readyToApply = false
		s.pendingDrain = false
	} else if err != nil && !s.maintenance {
		s.message = "A worker profile stopped; retrying"
	}
}

func persistInitialLimit(ctx context.Context, settings desktop.Settings, profile desktop.Profile) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body, err := json.Marshal(struct {
		Max int `json:"maxActiveJobs"`
	}{profile.Jobs})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, settings.Domain+"/api/v1/worker/settings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+settings.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("workerName", profile.Name)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("worker settings returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *supervisor) pollQueue(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		var settings *desktop.Settings
		if s.state.Active != nil {
			value := *s.state.Active
			settings = &value
		}
		s.mu.Unlock()
		if settings != nil {
			qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			queue, err := desktop.FetchQueue(qctx, *settings)
			cancel()
			s.mu.Lock()
			if s.state.Active != nil && s.state.Active.Domain == settings.Domain && s.state.Active.Token == settings.Token {
				s.queue = queue
				s.queueError = ""
				if err != nil {
					s.queueError = "unavailable"
					if errors.Is(err, desktop.ErrQueueUnsupported) {
						s.queueError = "unsupported"
					}
				}
			}
			s.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *supervisor) serve(ctx context.Context, owner string) error {
	if _, err := windows.StringToSid(owner); err != nil {
		return err
	}
	securityDescriptor := "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + owner + ")"
	if s.systemOwner {
		securityDescriptor = "O:SY" + securityDescriptor
	}
	listener, err := winio.ListenPipe(pipePrefix+owner+s.pipeSuffix, &winio.PipeConfig{SecurityDescriptor: securityDescriptor})
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() { <-ctx.Done(); _ = listener.Close() }()
	for {
		c, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleConnection(c)
	}
}

func (s *supervisor) handleConnection(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	var req desktop.Request
	decoder := json.NewDecoder(bufio.NewReader(io.LimitReader(c, 65536)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		_ = json.NewEncoder(c).Encode(desktop.Response{Error: "Invalid request"})
		return
	}
	_ = json.NewEncoder(c).Encode(s.handle(req))
}

type handler struct {
	owner       string
	dir, worker string
}

func (h *handler) Execute(_ []string, changes <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	state, err := desktop.Load(h.dir)
	if err != nil {
		return false, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &supervisor{dir: h.dir, worker: h.worker, state: state, systemOwner: true, children: map[string]*exec.Cmd{}, done: map[string]chan error{}}
	if err := s.initializeUpdates(ctx, h.owner); err != nil {
		return false, 1
	}
	ready := make(chan error, 1)
	go func() { ready <- s.serve(ctx, h.owner) }()
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); s.run(ctx) }()
	go s.pollQueue(ctx)
	statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-ready:
			if err != nil {
				cancel()
				<-workerDone
				return false, 1
			}
		case change := <-changes:
			switch change.Cmd {
			case svc.Interrogate:
				statuses <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				cancel()
				<-workerDone
				return false, 0
			}
		}
	}
}

func runConsole(dir, worker string) error {
	owner, err := currentSID()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	state, err := desktop.Load(dir)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	supervisor := &supervisor{dir: dir, worker: worker, state: state, pipeSuffix: os.Getenv("TRANSCODERD_TEST_PIPE_SUFFIX"), testCancel: cancel, children: map[string]*exec.Cmd{}, done: map[string]chan error{}}
	done := make(chan struct{})
	go func() { defer close(done); supervisor.run(ctx) }()
	go supervisor.pollQueue(ctx)
	err = supervisor.serve(ctx, owner)
	cancel()
	<-done
	return err
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--close-trays" {
		if err := managed.CloseInstalledTrays(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--record-installation" {
		if err := recordInstallation(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--install-owner" {
		if !windows.GetCurrentProcessToken().IsElevated() {
			os.Exit(1)
		}
		i, err := managed.ReadInstallation(dataDir())
		if err != nil || i.Owner != os.Args[2] {
			fmt.Fprintln(os.Stderr, "Installation identity mismatch")
			os.Exit(1)
		}
		if err = install(i.Owner); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "--console" {
		if err := runConsole(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--start" {
		if err := startService(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "--prepare-upgrade" || os.Args[1] == "--prepare-upgrade-wait") {
		if err := prepareUpgrade(os.Args[1] == "--prepare-upgrade-wait"); err != nil {
			if os.Args[1] == "--prepare-upgrade-wait" {
				fmt.Fprintln(os.Stdout, "Setup did not replace files. Accepted work can finish. Admission may remain held; retry Setup when idle, or restart the service after all jobs finish to resume normal work.")
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--install" {
		sid, err := currentSID()
		if err == nil {
			err = install(sid)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--uninstall" {
		if err := uninstall(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 3 || os.Args[1] != "--run" {
		fmt.Fprintln(os.Stderr, "usage: service --install|--uninstall|--run <owner SID>")
		os.Exit(2)
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	h := &handler{owner: os.Args[2], dir: dataDir(), worker: filepath.Join(filepath.Dir(exe), "transcoderd-worker.exe")}
	if err = svc.Run(serviceName, h); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Resolve ownership from live worker snapshots, never a caller-supplied profile.
func encodeOwner(profiles map[string]control.AdmissionStatus, jobID string) (string, error) {
	owner := ""
	for profile, status := range profiles {
		for _, job := range status.Jobs {
			if job.JobID == jobID && job.Phase == "FFMPEG" && job.CanPause {
				if owner != "" && owner != profile {
					return "", errors.New("encoding ownership is ambiguous; refresh and retry")
				}
				owner = profile
			}
		}
	}
	if owner == "" {
		return "", control.ErrJobNotPausable
	}
	return owner, nil
}
