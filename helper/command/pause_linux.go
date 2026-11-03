package command

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func openPausableProcess(process *os.Process) (*pausableProcess, error) {
	return &pausableProcess{active: func() (bool, error) {
		var info unix.Siginfo
		err := unix.Waitid(unix.P_PID, process.Pid, &info, unix.WEXITED|unix.WNOWAIT|unix.WNOHANG, nil)
		return info.Signo == 0, err
	}, setPaused: func(paused bool) error {
		signal := syscall.SIGCONT
		if paused {
			signal = syscall.SIGSTOP
		}
		// The group belongs to this command and remains owned until before Wait.
		err := syscall.Kill(-process.Pid, signal)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil || !paused {
			return err
		}
		if err := awaitProcessStop(process.Pid); err != nil {
			// SIGSTOP may still be pending. Undo it before reporting a failed pause.
			resumeErr := syscall.Kill(-process.Pid, syscall.SIGCONT)
			if errors.Is(resumeErr, syscall.ESRCH) {
				resumeErr = nil
			}
			return errors.Join(err, resumeErr)
		}
		return nil
	}, close: func() {}}, nil
}

func awaitProcessStop(pid int) error {
	deadline := time.Now().Add(time.Second)
	for {
		var info unix.Siginfo
		// WNOWAIT observes the group stop or an intervening exit without consuming
		// the exit status that os/exec must reap. Kill(SIGSTOP) alone is asynchronous.
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WSTOPPED|unix.WEXITED|unix.WNOWAIT|unix.WNOHANG, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("confirm encoding process stop: %w", err)
		}
		if info.Signo != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("encoding process did not acknowledge RAM pause within one second")
		}
		time.Sleep(time.Millisecond)
	}
}
