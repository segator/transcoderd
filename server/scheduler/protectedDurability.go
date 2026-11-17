package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// syncProtectedDirectory fails closed on filesystems or platforms that cannot
// make a namespace change durable. A committed DB record must never outlive
// the stage or final link it describes.
func syncProtectedDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return fmt.Errorf("open protected publication directory: %w", err)
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil {
		return fmt.Errorf("stat protected publication directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("protected publication path is not a directory")
	}
	if err := syncProtectedDirectoryHandle(directory); err != nil {
		return fmt.Errorf("sync protected publication directory: %w", err)
	}
	return nil
}

func syncProtectedCleanup(rootPath, stagingPath string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	dir := filepath.Dir(stagingPath)
	if err := syncProtectedDirectory(root, dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncProtectedDirectory(root, filepath.Dir(dir))
}

func syncProtectedFinal(rootPath, finalPath string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(finalPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open protected final output: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync protected final output: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncProtectedDirectory(root, filepath.Dir(finalPath))
}
