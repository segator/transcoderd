//go:build !windows

package command

import "os/exec"

func prepareProcess(_ *exec.Cmd) (func() error, func(), error) {
	return func() error { return nil }, func() {}, nil
}
