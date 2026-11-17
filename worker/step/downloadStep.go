package step

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/avast/retry-go"
	"github.com/google/uuid"
)

var errJobNotFound = errors.New("job Not found")

type DownloadStepExecutor struct {
	workerName    string
	BaseDomainURL string
	protected     protectedTransfers
}

func NewDownloadStepExecutor(workerName string, baseDomainUrl string, options ...ExecutorOption) *Executor {
	downloadStep := &DownloadStepExecutor{
		workerName:    workerName,
		BaseDomainURL: baseDomainUrl,
	}
	executor := NewStepExecutor(model.DownloadNotification, downloadStep.actions, options...)
	downloadStep.protected = executor.protected
	return executor
}

func (d *DownloadStepExecutor) actions(jobContext *job.Context) []Action {
	return []Action{
		{
			Execute: func(ctx context.Context, stepTracker Tracker) error {
				videoData, err := d.download(ctx, stepTracker, jobContext)
				if err != nil {
					return err
				}
				jobContext.Source = videoData
				return nil
			},
			Id: jobContext.JobId.String(),
		},
	}

}

func (d *DownloadStepExecutor) download(ctx context.Context, tracker Tracker, jobContext *job.Context) (*job.VideoData, error) {
	if jobContext.Assignment != nil {
		return d.downloadProtected(ctx, tracker, jobContext)
	}
	var sourceFilePath string
	var sourceChecksum string
	var originalName string
	var fileSize int64
	logger := tracker.Logger()
	err := retry.Do(func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", d.GetDownloadURL(jobContext.JobId), nil)
		if err != nil {
			return err
		}
		req.Header.Set("workerName", d.workerName)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusNotFound {
			return errJobNotFound
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("not 200 respose in download code %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		tracker.SetTotal(contentLength)
		if err != nil {
			return err
		}
		contentDisposition := resp.Header.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err != nil {
			return err
		}
		originalName = params["filename"]
		if originalName != "" {
			// Older servers encoded the entire filename with QueryEscape instead
			// of using the RFC-compliant filename* parameter.
			if !strings.Contains(strings.ToLower(contentDisposition), "filename*=") {
				originalName, err = url.QueryUnescape(originalName)
				if err != nil {
					return fmt.Errorf("decode downloaded filename: %w", err)
				}
			}
			originalName = filepath.Base(originalName)
		}
		if named, ok := tracker.(interface{ SetDisplayName(string) }); ok && originalName != "" {
			named.SetDisplayName(originalName)
		}

		sourceFilePath = filepath.Join(jobContext.WorkingDir, fmt.Sprintf("%s%s", jobContext.JobId.String(), filepath.Ext(params["filename"])))
		downloadFile, err := os.Create(sourceFilePath)
		if err != nil {
			return err
		}

		defer downloadFile.Close()

		reader := NewProgressTrackStream(tracker, resp.Body)

		fileSize, err = io.Copy(downloadFile, reader)
		if err != nil {
			return err
		}
		if fileSize != contentLength {
			return fmt.Errorf("file size error on download source:%d downloaded:%d", contentLength, fileSize)
		}
		sourceChecksum = hex.EncodeToString(reader.SumSha())
		bodyString, err := d.getChecksum(jobContext, logger)
		if err != nil {
			return err
		}

		if sourceChecksum != bodyString {
			return fmt.Errorf("checksum error on download source:%s downloaded:%s", bodyString, sourceChecksum)
		}
		// A completed download can later be parked across a process restart.
		// Its data must reach durable storage before a snapshot can reference it.
		if err := downloadFile.Sync(); err != nil {
			return fmt.Errorf("sync downloaded source: %w", err)
		}
		if err := downloadFile.Close(); err != nil {
			return fmt.Errorf("close downloaded source: %w", err)
		}

		tracker.UpdateValue(contentLength)
		return nil
	}, retry.Delay(time.Second*5),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(180), // 15 min
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			logger.Errorf("Error on downloading job %v", err)
		}),
		retry.RetryIf(func(err error) bool {
			return !errors.Is(err, context.Canceled) && !errors.Is(err, errJobNotFound)
		}))
	if err != nil {
		return nil, err
	}

	return downloadedVideoData(ctx, sourceFilePath, originalName, sourceChecksum, fileSize)
}

func downloadedVideoData(ctx context.Context, sourceFilePath, originalName, sourceChecksum string, fileSize int64) (*job.VideoData, error) {
	ffprobeData, err := ffmpeg.ExtractFFProbeData(ctx, sourceFilePath)
	if err != nil {
		return nil, err
	}

	normalizedFFProbeData, err := ffmpeg.NormalizeFFProbeData(ffprobeData)
	if err != nil {
		return nil, err
	}

	return &job.VideoData{
		FilePath:     sourceFilePath,
		OriginalName: originalName,
		Checksum:     sourceChecksum,
		Size:         fileSize,
		FFProbeData:  normalizedFFProbeData,
	}, nil
}

func (d *DownloadStepExecutor) downloadProtected(ctx context.Context, tracker Tracker, j *job.Context) (*job.VideoData, error) {
	if d.protected == nil || j.Assignment == nil {
		return nil, errors.New("protected download is not configured")
	}
	var path, checksum, name string
	var size int64
	err := retry.Do(func() error {
		download, err := d.protected.Download(ctx, *j.Assignment)
		if err != nil {
			return err
		}
		defer download.Body.Close()
		if download.Size <= 0 {
			return errors.New("protected download has invalid size")
		}
		name = download.Name
		path = filepath.Join(j.WorkingDir, j.JobId.String()+filepath.Ext(name))
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		tracker.SetTotal(download.Size)
		if named, ok := tracker.(interface{ SetDisplayName(string) }); ok && name != "" {
			named.SetDisplayName(name)
		}
		reader := NewProgressTrackStream(tracker, download.Body)
		size, err = io.Copy(file, reader)
		if err != nil {
			return err
		}
		if size != download.Size {
			return fmt.Errorf("protected download size mismatch: expected %d, received %d", download.Size, size)
		}
		checksum = hex.EncodeToString(reader.SumSha())
		serverChecksum, err := d.protected.Checksum(ctx, *j.Assignment)
		if err != nil {
			return err
		}
		if !strings.EqualFold(checksum, serverChecksum) {
			return errors.New("protected download checksum mismatch")
		}
		if err := file.Sync(); err != nil {
			return fmt.Errorf("sync protected download: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close protected download: %w", err)
		}
		tracker.UpdateValue(size)
		return nil
	}, retry.Context(ctx), retry.Delay(5*time.Second), retry.DelayType(retry.FixedDelay), retry.Attempts(180), retry.LastErrorOnly(true), retry.RetryIf(protectedTransferRetryable))
	if err != nil {
		return nil, err
	}
	return downloadedVideoData(ctx, path, name, checksum, size)
}

func protectedTransferRetryable(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, serverclient.ErrProtectedConflict) && !errors.Is(err, serverclient.ErrProtectedUnavailable)
}

func (d *DownloadStepExecutor) getChecksum(jobContext *job.Context, logger console.LeveledLogger) (string, error) {
	var bodyString string
	err := retry.Do(func() error {
		respsha256, err := http.Get(d.GetChecksumURL(jobContext.JobId))
		if err != nil {
			return err
		}
		defer respsha256.Body.Close()
		if respsha256.StatusCode != http.StatusOK {
			return fmt.Errorf("not 200 respose in sha265 code %d", respsha256.StatusCode)
		}

		bodyBytes, err := io.ReadAll(respsha256.Body)
		if err != nil {
			return err
		}
		bodyString = string(bodyBytes)
		return nil
	}, retry.Delay(time.Second*5),
		retry.Attempts(10),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			logger.Errorf("error %v on calculate checksum of downloaded job", err)
		}),
		retry.RetryIf(func(err error) bool {
			return !errors.Is(err, context.Canceled)
		}))
	if err != nil {
		return "", err
	}
	return bodyString, nil
}

func (d *DownloadStepExecutor) GetDownloadURL(id uuid.UUID) string {
	return fmt.Sprintf("%s%s?uuid=%s", d.BaseDomainURL, "/api/v1/download", id.String())
}

func (d *DownloadStepExecutor) GetChecksumURL(id uuid.UUID) string {
	return fmt.Sprintf("%s%s?uuid=%s", d.BaseDomainURL, "/api/v1/checksum", id.String())

}
