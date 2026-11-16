package serverclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

const (
	protectedRequestPath  = "/api/v2/worker/jobs/request"
	protectedLeasePath    = "/api/v2/worker/jobs/lease"
	protectedParkPath     = "/api/v2/worker/jobs/park"
	protectedEventPath    = "/api/v2/worker/jobs/event"
	protectedProgressPath = "/api/v2/worker/jobs/progress"
)

var (
	ErrProtectedUnavailable = errors.New("protected worker protocol is unavailable")
	ErrProtectedConflict    = errors.New("protected assignment or lease conflicts with server state")
	ErrProtectedLeaseLate   = errors.New("protected lease response arrived too late")
)

// ProtectedClient keeps the per-worker credential out of URLs and error text.
// Callers persist their request IDs and notifications before invoking methods;
// a transport failure has an unknown outcome and must be retried verbatim.
type ProtectedClient struct {
	client     *ServerClient
	credential string
}

func (s *ServerClient) Protected(credential string) (*ProtectedClient, error) {
	if credential == "" || s.workerName == "" {
		return nil, errors.New("protected protocol requires a worker identity and credential")
	}
	return &ProtectedClient{client: s, credential: credential}, nil
}

func (p *ProtectedClient) post(ctx context.Context, path string, input any, output any) (int, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	req, err := p.client.requestWithContext(ctx, http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("workerName", p.client.workerName)
	req.Header.Set("X-Worker-Credential", p.credential)
	resp, err := p.client.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return resp.StatusCode, ErrProtectedUnavailable
	case http.StatusConflict:
		return resp.StatusCode, ErrProtectedConflict
	case http.StatusOK, http.StatusNoContent:
	default:
		return resp.StatusCode, fmt.Errorf("protected worker request failed: HTTP %d", resp.StatusCode)
	}
	if output != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(output); err != nil {
			return resp.StatusCode, fmt.Errorf("decode protected worker response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func (p *ProtectedClient) RequestJob(ctx context.Context, requestID uuid.UUID) (*model.ProtectedAssignment, error) {
	if requestID == uuid.Nil {
		return nil, errors.New("protected request identity is required")
	}
	var result model.ProtectedAssignment
	status, err := p.post(ctx, protectedRequestPath, struct {
		RequestID string `json:"requestId"`
	}{RequestID: requestID.String()}, &result)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent {
		return nil, ErrNoJobAvailable
	}
	if result.Id == uuid.Nil || result.Generation == 0 || result.Capability == "" || result.KeyID == "" {
		return nil, errors.New("server returned an invalid protected assignment")
	}
	return &result, nil
}

// AcquireLease returns a monotonic local deadline measured from before the
// request was sent. Network delay therefore cannot extend permission to run.
func (p *ProtectedClient) AcquireLease(ctx context.Context, claim model.AssignmentClaim) (time.Time, error) {
	started := time.Now()
	var result struct {
		LeaseSeconds int `json:"leaseSeconds"`
	}
	status, err := p.post(ctx, protectedLeasePath, claim, &result)
	if err != nil {
		return time.Time{}, err
	}
	if status != http.StatusOK || result.LeaseSeconds < 10 || result.LeaseSeconds > 300 {
		return time.Time{}, errors.New("server returned an invalid protected lease")
	}
	deadline := started.Add(time.Duration(result.LeaseSeconds) * time.Second)
	if !time.Now().Before(deadline) {
		return time.Time{}, ErrProtectedLeaseLate
	}
	return deadline, nil
}

func (p *ProtectedClient) Park(ctx context.Context, claim model.AssignmentClaim) error {
	status, err := p.post(ctx, protectedParkPath, claim, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return errors.New("server did not confirm protected parking")
	}
	return nil
}

func (p *ProtectedClient) PublishEvent(ctx context.Context, claim model.AssignmentClaim, event model.TaskEventType) error {
	status, err := p.post(ctx, protectedEventPath, struct {
		Claim model.AssignmentClaim `json:"claim"`
		Event model.TaskEventType   `json:"event"`
	}{Claim: claim, Event: event}, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return errors.New("server did not acknowledge protected event")
	}
	return nil
}

func (p *ProtectedClient) PublishProgress(ctx context.Context, claim model.AssignmentClaim, progress model.TaskProgressType, sequence uint64) error {
	status, err := p.post(ctx, protectedProgressPath, struct {
		Claim    model.AssignmentClaim  `json:"claim"`
		Progress model.TaskProgressType `json:"progress"`
		Sequence uint64                 `json:"sequence"`
	}{Claim: claim, Progress: progress, Sequence: sequence}, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return errors.New("server did not acknowledge protected progress")
	}
	return nil
}
