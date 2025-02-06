package step

import (
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
	"transcoder/model"
	"transcoder/worker/job"
)

type UploadStepExecutor struct {
	BaseDomainURL string
	workerName    string
}

func NewUploadStepExecutor(workerName string, baseDomainUrl string, options ...ExecutorOption) *Executor {
	uploadStep := &UploadStepExecutor{
		workerName:    workerName,
		BaseDomainURL: baseDomainUrl,
	}
	return NewStepExecutor(model.UploadNotification, uploadStep.actions, options...)
}

func (u *UploadStepExecutor) actions(jobContext *job.Context) []Action {
	return []Action{
		{
			Execute: func(ctx context.Context, stepTracker Tracker) error {
				return u.upload(ctx, stepTracker, jobContext)
			},
			Id: jobContext.JobId.String(),
		},
	}

}

func (u *UploadStepExecutor) upload(ctx context.Context, tracker Tracker, jobContext *job.Context) error {
	return retry.Do(func() error {
		tracker.UpdateValue(0)
		encodedFile, err := os.Open(jobContext.Target.FilePath)
		if err != nil {
			return err
		}
		defer encodedFile.Close()
		fi, _ := encodedFile.Stat()
		fileSize := fi.Size()
		tracker.SetTotal(fileSize)

		reader := NewProgressTrackStream(tracker, encodedFile)

		client := &http.Client{}
		req, err := http.NewRequestWithContext(ctx, "POST", u.GetUploadURL(jobContext.JobId), reader)
		if err != nil {
			return err
		}
		req.ContentLength = fileSize
		req.Body = reader
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(reader), nil
		}
		req.Header.Set("workerName", u.workerName)
		req.Header.Add("checksum", jobContext.Target.Checksum)
		req.Header.Add("Content-Type", "application/octet-stream")
		req.Header.Add("Content-Length", strconv.FormatInt(fileSize, 10))
		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != 201 {
			return fmt.Errorf("invalid status Code %d", resp.StatusCode)
		}
		tracker.UpdateValue(fileSize)
		return nil
	}, retry.Delay(time.Second*5),
		retry.RetryIf(func(err error) bool {
			return !errors.Is(err, context.Canceled)
		}),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(17280),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			tracker.Logger().Errorf("Error on uploading job %v", err)
		}))
}

func (u *UploadStepExecutor) GetUploadURL(id uuid.UUID) string {
	return fmt.Sprintf("%s%s?uuid=%s", u.BaseDomainURL, "/api/v1/upload", id.String())
}
