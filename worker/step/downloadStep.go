package step

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"

	"github.com/avast/retry-go"
	"github.com/google/uuid"
)

var errJobNotFound = errors.New("job Not found")

type DownloadStepExecutor struct {
	workerName    string
	BaseDomainURL string
}

func NewDownloadStepExecutor(workerName string, baseDomainUrl string, options ...ExecutorOption) *Executor {
	downloadStep := &DownloadStepExecutor{
		workerName:    workerName,
		BaseDomainURL: baseDomainUrl,
	}
	return NewStepExecutor(model.DownloadNotification, downloadStep.actions, options...)
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
	var sourceFilePath string
	var sourceChecksum string
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
		_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
		if err != nil {
			return err
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

	ffprobeData, err := ffmpeg.ExtractFFProbeData(ctx, sourceFilePath)
	if err != nil {
		return nil, err
	}

	normalizedFFProbeData, err := ffmpeg.NormalizeFFProbeData(ffprobeData)
	if err != nil {
		return nil, err
	}

	return &job.VideoData{
		FilePath:    sourceFilePath,
		Checksum:    sourceChecksum,
		Size:        fileSize,
		FFProbeData: normalizedFFProbeData,
	}, nil
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
