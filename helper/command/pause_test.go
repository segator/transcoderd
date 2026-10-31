package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRAMProcessChild(t *testing.T) {
	path := os.Getenv("TRANSCODERD_TEST_PAUSE_CHILD")
	if path == "" {
		return
	}
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(2)
		}
		_, _ = f.Write([]byte("x"))
		_ = f.Close()
		time.Sleep(10 * time.Millisecond)
	}
}

func startPauseChild(t *testing.T, controller *PauseController, path string, key ...string) (context.CancelFunc, <-chan error) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("RAM pause requires Linux or Windows")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewCommand(exe, "-test.run=^TestRAMProcessChild$").AddEnv("TRANSCODERD_TEST_PAUSE_CHILD="+path).SetPauseController(controller, key...).RunWithContext(ctx)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("child did not terminate")
		}
	})
	return cancel, done
}

func pauseFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
func awaitPauseCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for process state")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRAMPauseStopsResumesAndCancelsOwnedProcess(t *testing.T) {
	controller := &PauseController{}
	path := filepath.Join(t.TempDir(), "progress")
	cancel, _ := startPauseChild(t, controller, path)
	awaitPauseCondition(t, func() bool { return pauseFileSize(path) >= 3 })
	for range 3 {
		if err := controller.SetPaused(true); err != nil {
			t.Fatal(err)
		}
		if err := controller.SetPaused(true); err != nil {
			t.Fatal(err)
		}
		before := pauseFileSize(path)
		time.Sleep(100 * time.Millisecond)
		if got := pauseFileSize(path); got != before {
			t.Fatalf("paused progress = %d, want %d", got, before)
		}
		if got := controller.Suspended(); got != 1 {
			t.Fatalf("Suspended() = %d, want 1", got)
		}
		if err := controller.SetPaused(false); err != nil {
			t.Fatal(err)
		}
		awaitPauseCondition(t, func() bool { return pauseFileSize(path) > before })
	}
	if err := controller.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	cancel()
	awaitPauseCondition(t, func() bool { return controller.Suspended() == 0 })
}

func TestRAMPauseHoldsStartsUntilResume(t *testing.T) {
	controller := &PauseController{}
	if err := controller.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "progress")
	startPauseChild(t, controller, path)
	time.Sleep(100 * time.Millisecond)
	if got := pauseFileSize(path); got != 0 {
		t.Fatalf("progress before resume = %d, want 0", got)
	}
	if err := controller.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	awaitPauseCondition(t, func() bool { return pauseFileSize(path) > 0 })
}

func TestRAMPauseWaitingStartHonorsCancellation(t *testing.T) {
	controller := &PauseController{}
	_ = controller.SetPaused(true)
	path := filepath.Join(t.TempDir(), "progress")
	cancel, _ := startPauseChild(t, controller, path)
	cancel()
	// Cleanup waits for return; no child should ever be launched.
	time.Sleep(30 * time.Millisecond)
	if got := pauseFileSize(path); got != 0 {
		t.Fatalf("canceled pending progress = %d, want 0", got)
	}
}

func TestRAMPauseFailureDoesNotReportSuccess(t *testing.T) {
	want := errors.New("suspend denied")
	controller := &PauseController{processes: map[*os.Process]*pausableProcess{{}: {setPaused: func(bool) error { return want }, close: func() {}}}}
	if err := controller.SetPaused(true); !errors.Is(err, want) {
		t.Fatalf("SetPaused() error = %v, want %v", err, want)
	}
	if got := controller.Suspended(); got != 0 {
		t.Fatalf("Suspended() = %d after failure, want 0", got)
	}
}

func TestRAMPauseDoesNotSuspendUnregisteredProcess(t *testing.T) {
	controller := &PauseController{}
	owned := filepath.Join(t.TempDir(), "owned")
	other := filepath.Join(t.TempDir(), "other")
	startPauseChild(t, controller, owned)
	startPauseChild(t, nil, other)
	awaitPauseCondition(t, func() bool { return pauseFileSize(owned) > 2 && pauseFileSize(other) > 2 })
	if err := controller.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	beforeOwned, beforeOther := pauseFileSize(owned), pauseFileSize(other)
	time.Sleep(100 * time.Millisecond)
	if got := pauseFileSize(owned); got != beforeOwned {
		t.Fatalf("owned process progressed %d -> %d", beforeOwned, got)
	}
	if got := pauseFileSize(other); got <= beforeOther {
		t.Fatalf("unregistered process did not progress: %d -> %d", beforeOther, got)
	}
}

func TestRAMPauseStartExitRaces(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("requires Linux or Windows")
	}
	controller := &PauseController{}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	toggleDone := make(chan error, 1)
	go func() {
		for range 30 {
			if err := controller.SetPaused(true); err != nil {
				toggleDone <- err
				return
			}
			if err := controller.SetPaused(false); err != nil {
				toggleDone <- err
				return
			}
		}
		toggleDone <- nil
	}()
	for range 10 {
		if _, err := NewCommand(exe, "-test.run=^TestNewAllowedCodesOption$").AddEnv("GORACE=atexit_sleep_ms=0").SetPauseController(controller).RunWithContext(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := <-toggleDone; err != nil {
		t.Fatal(err)
	}
	if got := controller.Suspended(); got != 0 {
		t.Fatalf("suspended after completion=%d,want 0", got)
	}
}

func TestRAMPauseRollsBackPartiallyAppliedTransition(t *testing.T) {
	want := errors.New("second process denied suspension")
	attempts, active, rollbacks := 0, 0, 0
	transition := func(paused bool) error {
		if paused {
			attempts++
			if attempts == 2 {
				return want
			}
			active++
		} else {
			active--
			rollbacks++
		}
		return nil
	}
	controller := &PauseController{processes: map[*os.Process]*pausableProcess{
		{}: {setPaused: transition, close: func() {}},
		{}: {setPaused: transition, close: func() {}},
	}}
	if err := controller.SetPaused(true); !errors.Is(err, want) {
		t.Fatalf("SetPaused() error=%v,want %v", err, want)
	}
	if active != 0 || rollbacks != 1 || controller.Suspended() != 0 {
		t.Fatalf("partial suspension not rolled back: active=%d rollbacks=%d suspended=%d", active, rollbacks, controller.Suspended())
	}
}

func TestIndividualPauseComposesWithGeneralPause(t *testing.T) {
	controller := &PauseController{}
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	startPauseChild(t, controller, first, "first")
	startPauseChild(t, controller, second, "second")
	awaitPauseCondition(t, func() bool { return pauseFileSize(first) > 2 && pauseFileSize(second) > 2 })
	if err := controller.SetJobPaused("first", true); err != nil {
		t.Fatal(err)
	}
	a, b := pauseFileSize(first), pauseFileSize(second)
	time.Sleep(80 * time.Millisecond)
	if pauseFileSize(first) != a || pauseFileSize(second) <= b {
		t.Fatal("individual pause did not isolate the chosen process")
	}
	if err := controller.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	a, b = pauseFileSize(first), pauseFileSize(second)
	time.Sleep(80 * time.Millisecond)
	if pauseFileSize(first) != a || pauseFileSize(second) != b {
		t.Fatal("general pause did not stop both processes")
	}
	if err := controller.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	awaitPauseCondition(t, func() bool { return pauseFileSize(second) > b })
	if pauseFileSize(first) != a || !controller.PauseStates()["first"].Individual {
		t.Fatal("general Resume cleared an individual hold")
	}
	if err := controller.SetJobPaused("first", false); err != nil {
		t.Fatal(err)
	}
	awaitPauseCondition(t, func() bool { return pauseFileSize(first) > a })
	if err := controller.SetJobPaused("stale", true); !errors.Is(err, ErrEncodeNotRunning) {
		t.Fatalf("stale pause error=%v", err)
	}
	if err := controller.SetJobPaused("first", true); err != nil {
		t.Fatal(err)
	}
	if err := controller.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	if err := controller.SetJobPaused("first", false); err != nil {
		t.Fatal(err)
	}
	a = pauseFileSize(first)
	time.Sleep(80 * time.Millisecond)
	if pauseFileSize(first) != a || controller.PauseStates()["first"].Individual {
		t.Fatal("individual Resume bypassed the general hold")
	}
	if err := controller.ResumeAll(); err != nil {
		t.Fatal(err)
	}
	awaitPauseCondition(t, func() bool { return pauseFileSize(first) > a })
}
