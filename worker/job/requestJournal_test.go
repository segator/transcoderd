package job

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestPendingRequestSurvivesRestartUntilResolved(t *testing.T) {
	path := t.TempDir()
	lock, err := AcquireWorkspaceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := lock.PendingRequest()
	if err != nil || id == uuid.Nil {
		t.Fatalf("PendingRequest() = %v, %v", id, err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err = AcquireWorkspaceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if got, err := lock.PendingRequest(); err != nil || got != id {
		t.Fatalf("PendingRequest() after restart = %v, %v; want %v", got, err, id)
	}
	if err := lock.ResolveRequest(uuid.New()); err == nil {
		t.Fatal("ResolveRequest() accepted a different request identity")
	}
	if err := lock.ResolveRequest(id); err != nil {
		t.Fatal(err)
	}
	if got, err := lock.PendingRequest(); err != nil || got == id || got == uuid.Nil {
		t.Fatalf("PendingRequest() after resolution = %v, %v", got, err)
	}
}

func TestPendingRequestRejectsCorruptJournal(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, requestJournalName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := AcquireWorkspaceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.PendingRequest(); err == nil {
		t.Fatal("PendingRequest() accepted a corrupt journal")
	}
}
