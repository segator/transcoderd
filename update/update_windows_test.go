package update

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWindowsUpdateProcessHelper(t *testing.T) {
	marker := os.Getenv("TRANSCODER_UPDATE_HELPER")
	if marker == "" {
		return
	}
	if err := os.WriteFile(marker, []byte("started"), 0o600); err != nil {
		os.Exit(2)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestWindowsBackupCleanupPreservesUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "worker.exe")
	owned := filepath.Join(dir, backupPrefix(target)+uuid.NewString()+".old")
	unrelated := filepath.Join(dir, backupPrefix(target)+"notes.old")
	directory := filepath.Join(dir, backupPrefix(target)+uuid.NewString()+".old")
	for _, path := range []string{owned, unrelated} {
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	cleanupBinaryBackups(target)
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("owned backup was not removed: %v", err)
	}
	for _, path := range []string{unrelated, directory} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated path was removed: %v", err)
		}
	}
}

func TestWindowsUpdateReplacesRunningExecutable(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "worker.exe")
	if err := os.WriteFile(target, data, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "started")
	process := exec.Command(target, "-test.run=^TestWindowsUpdateProcessHelper$")
	process.Env = append(os.Environ(), "TRANSCODER_UPDATE_HELPER="+marker)
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	replacement := append(append([]byte{}, data...), []byte("updated executable overlay")...)
	if err := applyBinaryUpdate(target, bytes.NewReader(replacement)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, replacement) {
		t.Fatal("updated executable does not match replacement")
	}
	second := append(append([]byte{}, data...), []byte("second executable overlay")...)
	if err := applyBinaryUpdate(target, bytes.NewReader(second)); err != nil {
		t.Fatalf("second update while original process remains running: %v", err)
	}
	got, err = os.ReadFile(target)
	if err != nil || !bytes.Equal(got, second) {
		t.Fatalf("second updated executable does not match replacement: %v", err)
	}
}
