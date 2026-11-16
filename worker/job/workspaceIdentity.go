package job

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
)

const workspaceIdentityFile = ".workspace-id"

// VerifyPersistentIdentity checks an operator-provisioned volume marker. It
// never creates one: an empty replacement volume must fail closed after a pod
// restart instead of being mistaken for the original durable workspace.
func (w *WorkspaceLock) VerifyPersistentIdentity(expected uuid.UUID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || expected == uuid.Nil {
		return errors.New("persistent workspace identity is unavailable")
	}
	root, err := os.OpenRoot(w.path)
	if err != nil {
		return fmt.Errorf("open persistent workspace: %w", err)
	}
	defer root.Close()
	file, err := root.Open(workspaceIdentityFile)
	if err != nil {
		return fmt.Errorf("open persistent workspace identity: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128 {
		return errors.New("persistent workspace identity is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, 129))
	if err != nil {
		return fmt.Errorf("read persistent workspace identity: %w", err)
	}
	id, err := uuid.Parse(strings.TrimSpace(string(data)))
	if err != nil || id != expected {
		return errors.New("persistent workspace identity does not match configuration")
	}
	return nil
}
