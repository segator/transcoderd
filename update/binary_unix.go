//go:build !windows

package update

import "github.com/minio/selfupdate"

func binaryUpdateOptions(path string) (selfupdate.Options, error) {
	return selfupdate.Options{TargetPath: path}, nil
}

func cleanupBinaryBackups(_ string) {}
