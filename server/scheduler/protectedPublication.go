package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"transcoder/model"
)

// CompleteProtectedJob durably reserves publication before touching the final
// destination. Retries and recovery resume the same assignment generation.
func (r *RuntimeScheduler) CompleteProtectedJob(ctx context.Context, worker string, claim model.AssignmentClaim, event model.TaskEventType) error {
	publication, err := r.repo.BeginProtectedPublication(ctx, worker, claim, event, r.config.DeleteSourceOnComplete)
	if err != nil {
		return err
	}
	return r.finalizeProtectedPublication(ctx, *publication)
}

func (r *RuntimeScheduler) finalizeProtectedPublication(ctx context.Context, publication model.ProtectedPublication) error {
	job, err := r.repo.GetJob(ctx, publication.JobID.String())
	if err != nil {
		return err
	}
	if !filepath.IsLocal(job.SourcePath) || !filepath.IsLocal(publication.StagingPath) {
		return errors.New("protected publication contains a non-local path")
	}
	if publication.State == "abandoned" {
		if err := removeUploadStagingFile(filepath.Join(r.config.SourcePath, publication.StagingPath)); err != nil {
			return fmt.Errorf("clean abandoned protected upload: %w", err)
		}
		return r.repo.ClearProtectedSourceDeletion(ctx, publication.JobID.String(), publication.Generation)
	}
	if publication.State == "completed" {
		return r.finishProtectedCleanup(ctx, publication, job)
	}
	if publication.State != "publishing" {
		return errors.New("protected publication is not ready")
	}
	root, err := os.OpenRoot(r.config.SourcePath)
	if err != nil {
		return err
	}
	file, err := root.Open(publication.StagingPath)
	if err != nil {
		_ = root.Close()
		return fmt.Errorf("open protected staged output: %w", err)
	}
	hash := sha256.New()
	n, readErr := io.Copy(hash, file)
	closeErr := file.Close()
	rootErr := root.Close()
	if err := errors.Join(readErr, closeErr, rootErr); err != nil {
		return fmt.Errorf("read protected staged output: %w", err)
	}
	if n != publication.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != publication.Checksum {
		return errors.New("protected staged output failed size or checksum validation")
	}
	stage := filepath.Join(r.config.SourcePath, publication.StagingPath)
	output, err := probeOutput(ctx, stage)
	if err != nil {
		return fmt.Errorf("probe protected staged output: %w", err)
	}
	mediaProbe := &model.MediaProbe{}
	if output.Format.Duration != "" {
		mediaProbe.DurationSeconds, _ = strconv.ParseFloat(output.Format.Duration, 64)
	}
	codec := ""
	for _, stream := range output.Streams {
		mediaProbe.Streams = append(mediaProbe.Streams, model.MediaStream{CodecType: stream.CodecType, CodecName: stream.CodecName, Width: stream.Width, Height: stream.Height, Language: stream.Tags.Language, Title: stream.Tags.Title})
		if codec == "" && stream.CodecType == "video" {
			codec = stream.CodecName
		}
	}
	if codec == "" {
		return errors.New("protected staged output has no video stream")
	}
	source := filepath.Join(r.config.SourcePath, job.SourcePath)
	desired := filepath.Join(r.config.SourcePath, outputNameForCodec(job.SourcePath, codec))
	final, err := linkOutputWithoutOverwrite(stage, desired, source)
	if err != nil {
		return err
	}
	if err := setSharedOutputPermissions(final); err != nil {
		return err
	}
	relative, err := filepath.Rel(r.config.SourcePath, final)
	if err != nil || !filepath.IsLocal(relative) {
		return errors.New("protected final output is outside the media root")
	}
	if err := r.repo.FinishProtectedPublication(ctx, publication, relative, n, mediaProbe); err != nil {
		return err
	}
	publication.State, publication.FinalPath = "completed", relative
	return r.finishProtectedCleanup(ctx, publication, job)
}

func (r *RuntimeScheduler) finishProtectedCleanup(ctx context.Context, publication model.ProtectedPublication, job *model.Job) error {
	if !filepath.IsLocal(publication.StagingPath) || !filepath.IsLocal(publication.FinalPath) || !filepath.IsLocal(job.SourcePath) {
		return errors.New("protected cleanup contains a non-local path")
	}
	stage := filepath.Join(r.config.SourcePath, publication.StagingPath)
	final := filepath.Join(r.config.SourcePath, publication.FinalPath)
	if !samePath(stage, final) {
		if err := removeUploadStagingFile(stage); err != nil {
			return fmt.Errorf("clean protected staged output: %w", err)
		}
	}
	if publication.DeleteSource {
		if err := os.Remove(filepath.Join(r.config.SourcePath, job.SourcePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete completed source: %w", err)
		}
	}
	return r.repo.ClearProtectedSourceDeletion(ctx, publication.JobID.String(), publication.Generation)
}

func (r *RuntimeScheduler) recoverProtectedPublications(ctx context.Context) error {
	pending, err := r.repo.ListPendingProtectedPublications(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, publication := range pending {
		if err := r.finalizeProtectedPublication(ctx, publication); err != nil {
			failures = append(failures, fmt.Errorf("recover publication %s generation %d: %w", publication.JobID, publication.Generation, err))
		}
	}
	return errors.Join(failures...)
}
