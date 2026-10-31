package config

import (
	"fmt"
	"runtime"
)

const (
	PauseFinish = "finish"
	PauseRAM    = "ram"
)

func ResolvePauseMode(mode string) (string, error) {
	if mode == "" {
		mode = PauseFinish
	}
	if mode != PauseFinish && mode != PauseRAM {
		return "", fmt.Errorf("pause mode must be finish or ram, got %q", mode)
	}
	if mode == PauseRAM && runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return "", fmt.Errorf("RAM pause requires Windows or Linux")
	}
	return mode, nil
}
