package update

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/selfupdate"
)

func binaryUpdateOptions(path string) (selfupdate.Options, error) {
	// The supervisor may keep the first renamed executable mapped for its whole
	// lifetime. A fixed .old name would prevent every subsequent Windows update.
	id, err := uuid.NewRandom()
	if err != nil {
		return selfupdate.Options{}, err
	}
	backup := filepath.Join(filepath.Dir(path), backupPrefix(path)+id.String()+".old")
	return selfupdate.Options{TargetPath: path, OldSavePath: backup}, nil
}

func backupPrefix(path string) string { return ".transcoderd-update-" + filepath.Base(path) + "-" }

func cleanupBinaryBackups(path string) {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return
	}
	prefix := backupPrefix(path)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".old") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".old")
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		// A backup still mapped by a running process cannot be removed; retry on
		// the next update or next supervisor start rather than failing the update.
		_ = os.Remove(filepath.Join(filepath.Dir(path), name))
	}
}
