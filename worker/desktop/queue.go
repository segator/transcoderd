package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"transcoder/model"
)

var ErrQueueUnsupported = errors.New("server does not support queue listing")

func FetchQueue(ctx context.Context, settings Settings) (*model.QueueSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.Domain+"/api/v1/jobs/queued", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrQueueUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("queue unavailable")
	}
	var queue model.QueueSnapshot
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&queue); err != nil {
		return nil, err
	}
	return &queue, nil
}
