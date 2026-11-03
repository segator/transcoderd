package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsProcessHelper(t *testing.T) {
	mode := os.Getenv("TRANSCODER_PROCESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "nested" {
		cmd := NewCommand(os.Args[0], "-test.run=^TestWindowsProcessHelper$")
		cmd.AddEnv("TRANSCODER_PROCESS_HELPER=parent")
		if _, err := cmd.RunWithContext(context.Background()); err != nil {
			os.Exit(5)
		}
		os.Exit(0)
	}
	if mode == "parent" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsProcessHelper$")
		cmd.Env = append(os.Environ(), "TRANSCODER_PROCESS_HELPER=child")
		if err := cmd.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Getenv("TRANSCODER_PROCESS_PID"), []byte(fmt.Sprint(cmd.Process.Pid)), 0o600); err != nil {
			os.Exit(4)
		}
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestWindowsCancellationTerminatesDescendants(t *testing.T) {
	for _, mode := range []string{"parent", "nested"} {
		t.Run(mode, func(t *testing.T) { testWindowsCancellationTerminatesDescendants(t, mode) })
	}
}

func testWindowsCancellationTerminatesDescendants(t *testing.T, mode string) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewCommand(os.Args[0], "-test.run=^TestWindowsProcessHelper$")
	cmd.AddEnv("TRANSCODER_PROCESS_HELPER=" + mode).AddEnv("TRANSCODER_PROCESS_PID=" + pidFile)
	done := make(chan error, 1)
	go func() { _, err := cmd.RunWithContext(ctx); done <- err }()
	deadline := time.Now().Add(10 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("parent exited before starting child: %v", err)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child did not start")
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	defer func() { _ = windows.TerminateProcess(process, 1) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("command did not stop")
	}
	if result, err := windows.WaitForSingleObject(process, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant remained alive: wait=%d err=%v", result, err)
	}
}

func TestWindowsCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewCommand("cmd.exe", "/c", "exit", "0").RunWithContext(ctx); err == nil {
		t.Fatal("canceled command succeeded")
	}
}
