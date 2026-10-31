//go:build !windows

package command

import (
	"os/exec"
	"syscall"
)

func prepareProcess(cmd *exec.Cmd, pausable bool) (func() error, func(), error) {
	if !pausable {
		return func() error { return nil }, func() {}, nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return func() error { return nil }, func() {}, nil
}
