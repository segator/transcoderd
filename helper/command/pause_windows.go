package command

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var (
	ntSuspendProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSuspendProcess")
	ntResumeProcess  = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
)

func openPausableProcess(process *os.Process) (*pausableProcess, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(process.Pid))
	if err != nil {
		return nil, fmt.Errorf("open encoding process for RAM pause: %w", err)
	}
	return &pausableProcess{active: func() (bool, error) {
		var exitCode uint32
		err := windows.GetExitCodeProcess(handle, &exitCode)
		return exitCode == 259, err
	}, setPaused: func(paused bool) error {
		var exitCode uint32
		if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
			return err
		}
		if exitCode != 259 {
			return nil
		} // STILL_ACTIVE
		procedure := ntResumeProcess
		if paused {
			procedure = ntSuspendProcess
		}
		status, _, _ := procedure.Call(uintptr(handle))
		if int32(status) < 0 {
			return fmt.Errorf("%s: NTSTATUS 0x%08x", procedure.Name, uint32(status))
		}
		return nil
	}, close: func() { _ = windows.CloseHandle(handle) }}, nil
}
