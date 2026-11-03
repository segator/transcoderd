package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"transcoder/model"
	"transcoder/server/library"
)

func (r *RuntimeScheduler) MediaSourceRoot() string { return r.config.SourcePath }
func (r *RuntimeScheduler) MediaMinimumSize() int64 { return r.config.MinFileSize }
func (r *RuntimeScheduler) GetMediaProbe(ctx context.Context, root, path string, size, modified int64) (*model.MediaProbe, error) {
	return r.repo.GetMediaProbe(ctx, root, path, size, modified)
}
func (r *RuntimeScheduler) PutMediaProbe(ctx context.Context, root, path string, size, modified int64, probe *model.MediaProbe) error {
	return r.repo.PutMediaProbe(ctx, root, path, size, modified, probe)
}
func (r *RuntimeScheduler) GetMediaJobStates(ctx context.Context, paths []string) (map[string]model.MediaJobState, error) {
	return r.repo.GetMediaJobStates(ctx, paths)
}

func (r *RuntimeScheduler) QueueMediaFile(ctx context.Context, file model.MediaFile, filters model.MediaFilters) (*model.Job, error) {
	r.jobIntakeMu.Lock()
	defer r.jobIntakeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if file.Root != "source" || !library.ValidPath(file.Path, false) || !library.IsVideo(file.Path) {
		return nil, library.ErrInvalidPath
	}
	root, err := os.OpenRoot(r.config.SourcePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := library.CheckPath(root, file.Path, false); err != nil {
		return nil, err
	}
	f, err := root.Open(file.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != file.Size || info.ModTime().UnixNano() != file.ModifiedNS {
		return nil, library.ErrChanged
	}
	if info.Size() < r.config.MinFileSize {
		return nil, fmt.Errorf("source is smaller than the server minimum")
	}
	if file.Metadata == nil || file.Metadata.Probe == nil {
		return nil, fmt.Errorf("reviewed source metadata is required")
	}
	request := &model.JobRequest{SourcePath: file.Path, SourceSize: file.Size, TargetPath: formatTargetName(file.Path), SourceProbe: file.Metadata.Probe, ForceFailed: filters.RetryFailed, ForceCanceled: filters.RetryCanceled}
	if request.TargetPath == file.Path {
		request.TargetPath = strings.TrimSuffix(file.Path, filepath.Ext(file.Path)) + "_encoded.mkv"
	}
	return r.scheduleJobRequestLocked(ctx, request)
}
