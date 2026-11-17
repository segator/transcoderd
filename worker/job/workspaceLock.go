package job

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrWorkspaceInUse = errors.New("worker workspace is already in use")

// WorkspaceLock keeps one worker process responsible for the durable state in
// a workspace. The lock is held by the OS and released on process exit; the
// lock file itself is deliberately not removed, avoiding inode replacement.
type WorkspaceLock struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func AcquireWorkspaceLock(path string) (*WorkspaceLock, error) {
	if path == "" {
		return nil, errors.New("workspace path is empty")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve worker workspace: %w", err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create worker workspace: %w", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open worker workspace: %w", err)
	}
	defer root.Close()
	file, err := root.OpenFile(".workspace.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock: %w", err)
	}
	if err := lockWorkspaceFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock worker workspace: %w", err)
	}
	return &WorkspaceLock{file: file, path: path}, nil
}

func (w *WorkspaceLock) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := errors.Join(unlockWorkspaceFile(w.file), w.file.Close())
	w.file = nil
	return err
}

func (w *WorkspaceLock) Path() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return "", errors.New("worker workspace lock is closed")
	}
	return w.path, nil
}
