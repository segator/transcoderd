package job

import (
	"errors"
	"testing"
)

func TestWorkspaceLockRejectsConcurrentOwner(t *testing.T) {
	path := t.TempDir()
	first, err := AcquireWorkspaceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireWorkspaceLock(path); !errors.Is(err, ErrWorkspaceInUse) {
		t.Fatalf("second workspace lock = %v, want ErrWorkspaceInUse", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireWorkspaceLock(path)
	if err != nil {
		t.Fatalf("workspace lock after release = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
