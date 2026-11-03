package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"transcoder/update/managed"
	"transcoder/worker/control"
	"transcoder/worker/desktop"
)

func manualUpgradeStatus() desktop.Status {
	return desktop.Status{Configured: true, Running: true, Profiles: map[string]control.AdmissionStatus{"cpu": {}, "gpu": {}}}
}

func TestManualUpgradeHoldsBothProfilesBeforeWaiting(t *testing.T) {
	var called []string
	status := manualUpgradeStatus()
	active, reserved, err := holdProfilesForManualUpgrade(status, func(profile string, request control.Request) (control.Response, error) {
		called = append(called, profile)
		if request.Command != "hold-admission" {
			t.Fatalf("unexpected mutation %q", request.Command)
		}
		if profile == "cpu" {
			return control.Response{Status: control.AdmissionStatus{ActiveJobs: 1}}, nil
		}
		return control.Response{Status: control.AdmissionStatus{ActiveJobs: 4, ReservedJobs: 1}}, nil
	})
	if err != nil || active != 5 || reserved != 1 || strings.Join(called, ",") != "cpu,gpu" {
		t.Fatalf("drain=%d,%d,%v calls=%v", active, reserved, err, called)
	}
}

func TestManualUpgradeUnknownActivityAndCompetingMaintenanceFailSafely(t *testing.T) {
	for _, name := range []string{"missing-cpu", "missing-gpu", "control-error", "accepting", "response-error", "automatic-update", "configuration"} {
		t.Run(name, func(t *testing.T) {
			status := manualUpgradeStatus()
			switch name {
			case "missing-cpu":
				delete(status.Profiles, "cpu")
			case "missing-gpu":
				delete(status.Profiles, "gpu")
			case "automatic-update":
				status.Update = &managed.Status{Pending: true}
			case "configuration":
				status.Pending = true
			}
			calls := 0
			_, _, err := holdProfilesForManualUpgrade(status, func(profile string, _ control.Request) (control.Response, error) {
				calls++
				if profile == "cpu" {
					switch name {
					case "control-error":
						return control.Response{}, errors.New("offline")
					case "accepting":
						return control.Response{Status: control.AdmissionStatus{Accepting: true}}, nil
					case "response-error":
						return control.Response{Error: "rejected"}, nil
					}
				}
				return control.Response{}, nil
			})
			if err == nil {
				t.Fatal("unsafe state accepted as idle")
			}
			if (name == "control-error" || name == "accepting" || name == "response-error") && calls != 2 {
				t.Fatal("first profile error skipped the other profile")
			}
			if (name == "automatic-update" || name == "configuration") && calls != 0 {
				t.Fatal("manual Setup competed with maintenance")
			}
		})
	}
}

func TestManualUpgradeIncludesQueuedEncodesAndReservations(t *testing.T) {
	active, reserved, err := holdProfilesForManualUpgrade(manualUpgradeStatus(), func(profile string, _ control.Request) (control.Response, error) {
		if profile == "cpu" {
			return control.Response{Status: control.AdmissionStatus{Jobs: []control.JobState{{State: "queued"}, {State: "queued"}}}}, nil
		}
		return control.Response{Status: control.AdmissionStatus{ActiveEncodes: 1, ReservedJobs: 1}}, nil
	})
	if err != nil || active != 3 || reserved != 1 {
		t.Fatalf("remaining=%d,%d,%v", active, reserved, err)
	}
	active, reserved, err = holdProfilesForManualUpgrade(desktop.Status{}, func(string, control.Request) (control.Response, error) {
		t.Fatal("unconfigured service used a worker")
		return control.Response{}, nil
	})
	if err != nil || active != 0 || reserved != 0 {
		t.Fatal("unconfigured service could not upgrade")
	}
}

func TestManualUpgradeWaitsThroughWorkAndReportsProgress(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	err := waitForManualUpgrade(context.Background(), func() (int, int, error) {
		calls++
		switch calls {
		case 1:
			return 3, 1, nil
		case 2:
			return 0, 1, nil
		default:
			return 0, 0, nil
		}
	}, time.Millisecond, &output)
	if err != nil || calls != 3 {
		t.Fatalf("wait=%v calls=%d", err, calls)
	}
	if !strings.Contains(output.String(), "3 accepted jobs") || !strings.Contains(output.String(), "All accepted work has finished") {
		t.Fatalf("progress=%s", output.String())
	}
}

func TestManualUpgradeWaitStopsOnCancellationTimeoutAndFailure(t *testing.T) {
	for _, name := range []string{"cancel", "cancel-idle", "timeout", "failure"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			calls := 0
			err := waitForManualUpgrade(ctx, func() (int, int, error) {
				calls++
				if name == "cancel" || name == "cancel-idle" {
					cancel()
				}
				if name == "cancel-idle" {
					return 0, 0, nil
				}
				if name == "failure" {
					return 0, 0, errors.New("worker unavailable")
				}
				return 1, 0, nil
			}, time.Millisecond, &bytes.Buffer{})
			if err == nil || calls == 0 {
				t.Fatal("wait succeeded despite interruption")
			}
			if (name == "cancel" || name == "cancel-idle") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if name == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		})
	}
}

func TestServicePreparationDrainsGPUWhileCPUBusy(t *testing.T) {
	service := &supervisor{state: desktop.Persisted{Active: &desktop.Settings{}}, children: map[string]*exec.Cmd{"cpu": {}, "gpu": {}}, shutdownPending: true}
	busy := true
	var called []string
	service.workerControl = func(profile string, request control.Request) (control.Response, error) {
		called = append(called, profile)
		if request.Command != "drain" {
			t.Fatal(request.Command)
		}
		status := control.AdmissionStatus{}
		if busy && profile == "cpu" {
			status.ActiveJobs = 1
		}
		return control.Response{Status: status}, nil
	}
	if err := service.prepareUpgradeLocked(); err == "" || strings.Join(called, ",") != "cpu,gpu" || service.maintenance || service.shutdownPending {
		t.Fatalf("busy preparation=%q calls=%v", err, called)
	}
	busy = false
	if err := service.prepareUpgradeLocked(); err != "" || !service.maintenance {
		t.Fatalf("idle preparation=%q", err)
	}
	delete(service.children, "gpu")
	if err := service.prepareUpgradeLocked(); err == "" || service.maintenance {
		t.Fatal("missing configured worker permitted replacement")
	}
}

func TestManualUpgradeParentStub(t *testing.T) {
	if os.Getenv("TRANSCODERD_INSTALLER_PARENT_STUB") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}

func TestManualUpgradeWaitCancelsWhenSetupProcessExits(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	parent := exec.Command(executable, "-test.run=^TestManualUpgradeParentStub$")
	parent.Env = append(os.Environ(), "TRANSCODERD_INSTALLER_PARENT_STUB=1")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Process.Kill(); _ = parent.Wait() }()
	ctx, cancel, err := installerProcessContext(uint32(parent.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal("live Setup canceled the wait")
	default:
	}
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("wait survived Setup exit")
	}
}
