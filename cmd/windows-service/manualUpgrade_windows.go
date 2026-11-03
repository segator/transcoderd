package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"transcoder/update/managed"
	"transcoder/worker/control"
	"transcoder/worker/desktop"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const manualUpgradeBusyMessage = "Jobs are still running. Wait for them to finish and try again."

func upgradeServiceStatus(owner string, request desktop.Request) (desktop.Response, error) {
	timeout := 3 * time.Second
	conn, err := winio.DialPipe(pipePrefix+owner, &timeout)
	if err != nil {
		return desktop.Response{}, fmt.Errorf("cannot contact the service: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return desktop.Response{}, err
	}
	var response desktop.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return response, err
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

func holdProfilesForManualUpgrade(status desktop.Status, call func(string, control.Request) (control.Response, error)) (int, int, error) {
	if status.Update != nil && status.Update.Pending {
		return 0, 0, errors.New("mandatory automatic update is pending; wait for it to finish before running Setup")
	}
	if status.Pending {
		return 0, 0, errors.New("configuration is pending; wait for it to finish before running Setup")
	}
	if !status.Configured && !status.Running {
		return 0, 0, nil
	}
	active, reserved := 0, 0
	var problems []error
	for _, profile := range []string{"cpu", "gpu"} {
		if _, ok := status.Profiles[profile]; !ok {
			problems = append(problems, fmt.Errorf("%s worker is unavailable; refusing to assume it is idle", profile))
			continue
		}
		response, err := call(profile, control.Request{Command: "hold-admission"})
		if err != nil {
			problems = append(problems, fmt.Errorf("hold %s admission: %w", profile, err))
			continue
		}
		if response.Error != "" {
			problems = append(problems, errors.New(response.Error))
			continue
		}
		if response.Status.Accepting {
			problems = append(problems, fmt.Errorf("%s worker did not confirm closed admission", profile))
			continue
		}
		// Include queued jobs and transient counter disagreement conservatively.
		active += max(int(response.Status.ActiveJobs), response.Status.ActiveEncodes, len(response.Status.Jobs))
		reserved += response.Status.ReservedJobs
	}
	return active, reserved, errors.Join(problems...)
}

func waitForManualUpgrade(ctx context.Context, probe func() (int, int, error), interval time.Duration, output io.Writer) error {
	fmt.Fprintln(output, "Waiting for accepted work before upgrading. The service will remain running until CPU and GPU are idle.")
	previousActive, previousReserved := -1, -1
	lastPrint := time.Time{}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("upgrade wait ended without stopping the service: %w", err)
		}
		active, reserved, err := probe()
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("upgrade wait ended without stopping the service: %w", err)
		}
		if active != previousActive || reserved != previousReserved || time.Since(lastPrint) >= 30*time.Second {
			fmt.Fprintf(output, "Remaining: %d accepted jobs, %d requests. New CPU/GPU assignments are blocked.\n", active, reserved)
			previousActive, previousReserved = active, reserved
			lastPrint = time.Now()
		}
		if active == 0 && reserved == 0 {
			fmt.Fprintln(output, "All accepted work has finished. Preparing service replacement.")
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("upgrade wait ended without stopping the service: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (s *supervisor) prepareUpgradeLocked() string {
	s.shutdownPending = false
	if s.updatePending() {
		return managed.LockedMessage
	}
	s.maintenance = true
	var problems []error
	busy := false
	for _, profile := range []string{"cpu", "gpu"} {
		if s.children[profile] == nil {
			if s.state.Active != nil {
				problems = append(problems, fmt.Errorf("%s worker is unavailable", profile))
			}
			continue
		}
		response, err := s.callWorker(profile, control.Request{Command: "drain"})
		if err != nil {
			problems = append(problems, fmt.Errorf("drain %s: %w", profile, err))
			continue
		}
		if response.Error != "" {
			problems = append(problems, errors.New(response.Error))
			continue
		}
		if response.Status.ActiveJobs != 0 || response.Status.ReservedJobs != 0 || response.Status.ActiveEncodes != 0 || len(response.Status.Jobs) > 0 {
			busy = true
		}
	}
	if len(problems) > 0 || busy {
		s.maintenance = false
		if len(problems) > 0 {
			return errors.Join(problems...).Error()
		}
		return manualUpgradeBusyMessage
	}
	return ""
}

// The extracted helper must not finish an upgrade after its Setup process exits.
func installerWaitContext() (context.Context, context.CancelFunc, error) {
	return installerProcessContext(uint32(os.Getppid()))
}

func installerProcessContext(pid uint32) (context.Context, context.CancelFunc, error) {
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, nil, fmt.Errorf("monitor Setup process: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	go func() {
		defer func() { _ = windows.CloseHandle(parent) }()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			result, err := windows.WaitForSingleObject(parent, 250)
			if err != nil || result != uint32(windows.WAIT_TIMEOUT) {
				cancel()
				return
			}
		}
	}()
	return ctx, cancel, nil
}
