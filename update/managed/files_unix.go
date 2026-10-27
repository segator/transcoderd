//go:build !windows

package managed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func ProtectDir(dir string) error {
	if err := CheckPath(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}
func CheckPath(path string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		s, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("update paths cannot contain symlinks: %s", current)
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}
func ReplaceFile(old, new string) error { return os.Rename(old, new) }
func AvailableSpace(path string, required int64) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return err
	}
	if uint64(stat.Bavail)*uint64(stat.Bsize) < uint64(required) {
		return errors.New("not enough update staging disk space")
	}
	return nil
}
