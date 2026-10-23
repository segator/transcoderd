package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
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
	root        *os.Root
	attemptName string
	stagingName string
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
	if u.root == nil || u.attemptName == "" || u.stagingName == "" {
		return errors.New("upload staging paths are not initialized")
	}
	if err := u.root.Link(u.attemptName, u.stagingName); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("publish upload staging file: %w", err)
		}
		existingChecksum, checksumErr := rootChecksum(u.root, u.stagingName)
		if checksumErr != nil {
			return fmt.Errorf("verify existing upload staging file: %w", checksumErr)
		}
		if existingChecksum != u.GetHash() {
			return errors.New("an upload with different contents is already staged for this job")
		}
	}
	return nil
}

func rootChecksum(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
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
	if u.root == nil {
		return nil
	}
	removeErr := u.root.Remove(u.attemptName)
	closeErr := u.root.Close()
	u.root = nil
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	return closeErr
}
