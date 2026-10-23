package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
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
	stagingDir  string
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
	if !filepath.IsLocal(u.stagingName) {
		return fmt.Errorf("invalid upload staging filename %q", u.stagingName)
	}
	return os.Remove(filepath.Join(u.stagingDir, u.stagingName))
}
