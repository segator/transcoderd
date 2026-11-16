//go:build !windows

package scheduler

import "os"

func syncProtectedDirectoryHandle(directory *os.File) error {
	return directory.Sync()
}
