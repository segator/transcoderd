//go:build !windows

package job

import (
	"fmt"
	"os"
	"path/filepath"
)

func replaceContextFile(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	// The file is already synced; persist the rename before acknowledging it.
	dir, err := os.Open(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("open context directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync context directory: %w", err)
	}
	return nil
}
