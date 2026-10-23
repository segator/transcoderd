package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"transcoder/model"
)

type PathChecksum struct {
	path     string
	checksum string
}
type JobStream struct {
	hasher            hash.Hash
	video             *model.Job
	path              string
	file              *os.File
	checksumPublisher chan PathChecksum
}

type UploadJobStream struct {
	*JobStream
	stagingRoot string
	finalPath   string
}

type DownloadJobStream struct {
	*JobStream
	FileSize int64
	FileName string
}

func (j *JobStream) hash(p []byte) (err error) {
	if j.hasher == nil {
		j.hasher = sha256.New()
	}
	if _, err := j.hasher.Write(p); err != nil {
		return err
	}
	return nil
}
func (j *JobStream) GetHash() string {
	return hex.EncodeToString(j.hasher.Sum(nil))
}
func (u *UploadJobStream) Write(p []byte) (n int, err error) {
	err = u.hash(p)
	if err != nil {
		return 0, err
	}
	return u.file.Write(p)
}
func (d *DownloadJobStream) Read(p []byte) (n int, err error) {
	readed, err := d.file.Read(p)
	if err != nil {
		return readed, err
	}
	err = d.hash(p[0:readed])
	if err != nil {
		return 0, err
	}
	return readed, err
}

func (d *DownloadJobStream) Size() int64 {
	return d.FileSize
}

func (d *DownloadJobStream) Name() string {
	return d.FileName
}

func (u *UploadJobStream) Close(pushChecksum bool) error {
	err := u.file.Sync()
	if err != nil {
		return err
	}
	return u.file.Close()
}

// Commit publishes a fully received, validated upload at its job staging path.
// Each HTTP upload attempt writes a separate temporary file, so cleanup from a
// failed retry cannot unlink a later successful attempt.
func (u *UploadJobStream) Commit() error {
	if u.stagingRoot == "" || u.finalPath == "" {
		return errors.New("upload staging root or destination is empty")
	}
	root, err := os.OpenRoot(u.stagingRoot)
	if err != nil {
		return fmt.Errorf("open upload staging root: %w", err)
	}
	defer root.Close()
	attemptRelative, err := filepath.Rel(u.stagingRoot, u.file.Name())
	if err != nil || !filepath.IsLocal(attemptRelative) {
		return errors.New("upload attempt path is outside the configured source root")
	}
	destinationRelative, err := filepath.Rel(u.stagingRoot, u.finalPath)
	if err != nil || !filepath.IsLocal(destinationRelative) || destinationRelative == "." {
		return errors.New("upload staging destination is outside the configured source root")
	}
	if filepath.Dir(attemptRelative) != filepath.Dir(destinationRelative) {
		return errors.New("upload attempt and staging destination must share a directory")
	}
	if err := root.Link(attemptRelative, destinationRelative); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("publish upload staging file: %w", err)
		}
		existing, openErr := root.Open(destinationRelative)
		if openErr != nil {
			return fmt.Errorf("open existing upload staging file: %w", openErr)
		}
		existingHash := sha256.New()
		if _, copyErr := io.Copy(existingHash, existing); copyErr != nil {
			_ = existing.Close()
			return fmt.Errorf("hash existing upload staging file: %w", copyErr)
		}
		if closeErr := existing.Close(); closeErr != nil {
			return fmt.Errorf("close existing upload staging file: %w", closeErr)
		}
		if hex.EncodeToString(existingHash.Sum(nil)) != u.GetHash() {
			return errors.New("an upload with different contents is already staged for this job")
		}
	}
	return nil
}

func (j *JobStream) Close(pushChecksum bool) error {
	err := j.file.Sync()
	if err != nil {
		return err
	}
	j.file.Close()
	if j.hasher != nil && pushChecksum {
		j.checksumPublisher <- PathChecksum{
			path:     j.path,
			checksum: j.GetHash(),
		}
	}
	return nil
}
func (u *UploadJobStream) Clean() error {
	return os.Remove(u.file.Name())
}
