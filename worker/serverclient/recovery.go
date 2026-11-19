package serverclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const startupRetryDelay = 5 * time.Second

func (s *ServerClient) WaitForInitialPresence(ctx context.Context) error {
	return retryStartupRequest(ctx, startupRetryDelay, "publish initial worker presence", func(attemptCtx context.Context) error {
		return s.PublishPingEventContext(attemptCtx)
	})
}

func (s *ServerClient) GetJobForRecovery(ctx context.Context, id uuid.UUID) (*model.Job, error) {
	var current *model.Job
	err := retryStartupRequest(ctx, startupRetryDelay, "check saved job "+id.String(), func(attemptCtx context.Context) error {
		req, err := s.requestWithContext(attemptCtx, http.MethodGet, "/api/v1/job/"+id.String(), nil)
		if err != nil {
			return err
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil
		case http.StatusOK:
			var snapshot model.Job
			if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
				return fmt.Errorf("decode job: %w", err)
			}
			if snapshot.Id != id || snapshot.Events.GetLatestPerNotificationType(model.JobNotification) == nil {
				return fmt.Errorf("job lookup returned incomplete state")
			}
			current = &snapshot
			return nil
		default:
			return fmt.Errorf("job lookup returned %s", resp.Status)
		}
	})
	return current, err
}

func retryStartupRequest(ctx context.Context, delay time.Duration, operation string, request func(context.Context) error) error {
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := request(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}

		log.WithError(err).Warnf("Could not %s; retrying", operation)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return ctx.Err()
}
