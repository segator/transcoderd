package job

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestPersistentWorkspaceIdentityFailsClosedOnReplacement(t *testing.T) {
	path := t.TempDir()
	lock, err := AcquireWorkspaceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	id := uuid.New()
	if err := lock.VerifyPersistentIdentity(id); err == nil {
		t.Fatal("missing workspace marker was accepted")
	}
	if err := os.WriteFile(filepath.Join(path, workspaceIdentityFile), []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lock.VerifyPersistentIdentity(id); err != nil {
		t.Fatal(err)
	}
	if err := lock.VerifyPersistentIdentity(uuid.New()); err == nil {
		t.Fatal("different workspace marker was accepted")
	}
}
