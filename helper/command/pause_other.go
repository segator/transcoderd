//go:build !windows && !linux

package command

import (
	"errors"
	"os"
)

func openPausableProcess(_ *os.Process) (*pausableProcess, error) {
	return &pausableProcess{setPaused: func(paused bool) error {
		if paused {
			return errors.New("RAM pause is supported only on Windows and Linux")
		}
		return nil
	}, close: func() {}}, nil
}
