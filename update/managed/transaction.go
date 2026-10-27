package managed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidateWorkspace keeps user data out of the tree that Setup replaces.
func ValidateWorkspace(install, workspace string) error {
	if workspace == "" {
		return nil
	}
	// Resolve existing ancestors too, including a junction above a new directory.
	resolve := func(path string) string {
		current := filepath.Clean(path)
		var suffix []string
		for {
			if resolved, err := filepath.EvalSymlinks(current); err == nil {
				for i := len(suffix) - 1; i >= 0; i-- {
					resolved = filepath.Join(resolved, suffix[i])
				}
				return resolved
			}
			parent := filepath.Dir(current)
			if parent == current {
				return filepath.Clean(path)
			}
			suffix = append(suffix, filepath.Base(current))
			current = parent
		}
	}
	root, work := strings.ToLower(resolve(install)), strings.ToLower(resolve(workspace))
	rel, err := filepath.Rel(root, work)
	if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return errors.New("working directory must be outside the application installation folder before updating")
	}
	return nil
}

type Transaction struct {
	Phase           string    `json:"phase"`
	Candidate       Candidate `json:"candidate"`
	InstallDir      string    `json:"installDir"`
	Owner           string    `json:"owner"`
	PreviousVersion string    `json:"previousVersion"`
	BackupFiles     []File    `json:"backupFiles,omitempty"`
	PID             uint32    `json:"pid,omitempty"`
	Started         time.Time `json:"started"`
	Error           string    `json:"error,omitempty"`
	BootTime        time.Time `json:"bootTime,omitempty"`
}

func TransactionPath(dir string) string { return filepath.Join(dir, "updates", "transaction.json") }
func LoadTransaction(dir string) (*Transaction, error) {
	b, err := os.ReadFile(TransactionPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 4*MaxManifestSize {
		return nil, errors.New("update transaction too large")
	}
	var t Transaction
	if err = json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	return &t, nil
}
func (t *Transaction) Active() bool {
	return t != nil && (t.Phase == "prepared" || t.Phase == "installing" || t.Phase == "verifying" || t.Phase == "recovering" || t.Phase == "restart-required")
}

func CopyTree(source, dest string) error {
	if err := CheckPath(source); err != nil {
		return err
	}
	if err := ProtectDir(dest); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = CheckPath(path); err != nil {
			return err
		}
		name, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, name)
		if d.IsDir() {
			return ProtectDir(target)
		}
		return CopyFile(path, target)
	})
}

func CopyFile(source, target string) error {
	if err := CheckPath(source); err != nil {
		return err
	}
	if err := CheckPath(target); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = ProtectDir(filepath.Dir(target)); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(target), ".copy-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err = protectFile(tmp); err != nil {
		return err
	}
	return ReplaceFile(tmp, target)
}

// RestoreTree removes files introduced by the failed installer before restoring
// the complete old installation. Configuration/workspaces live outside it.
func RestoreTree(backup, install string, files []File) error {
	if err := VerifyInstallation(backup, files); err != nil {
		return fmt.Errorf("verify rollback snapshot: %w", err)
	}
	if err := CheckPath(install); err != nil {
		return err
	}
	if err := os.RemoveAll(install); err != nil {
		return err
	}
	if err := CopyTree(backup, install); err != nil {
		return err
	}
	return VerifyInstallation(install, files)
}
