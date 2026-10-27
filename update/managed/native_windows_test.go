package managed

import (
	"os"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstallationMutexExcludesAnotherThread(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("machine installation mutex is tested only on an isolated runner")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handle, err := mutex()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	defer func() { _ = windows.ReleaseMutex(handle) }()
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		other, err := mutex()
		if err == nil {
			_ = windows.ReleaseMutex(other)
			_ = windows.CloseHandle(other)
		}
		done <- err
	}()
	if err = <-done; err == nil {
		t.Fatal("another thread acquired the installation mutex")
	}
}
