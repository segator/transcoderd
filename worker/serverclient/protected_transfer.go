package serverclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"

	"transcoder/model"

	"github.com/google/uuid"
)

const (
	protectedDownloadPath = "/api/v2/worker/jobs/download"
	protectedChecksumPath = "/api/v2/worker/jobs/checksum"
	protectedUploadPath   = "/api/v2/worker/jobs/upload"
)

type ProtectedDownload struct {
	Body io.ReadCloser
	Name string
	Size int64
}

func (p *ProtectedClient) transferRequest(ctx context.Context, method, path string, claim model.AssignmentClaim, body io.Reader) (*http.Request, error) {
	if claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || claim.InstanceID == uuid.Nil {
		return nil, errors.New("protected transfer requires a current assignment and process lease")
	}
	query := url.Values{"uuid": {claim.JobID.String()}}
	req, err := p.client.requestWithContext(ctx, method, path+"?"+query.Encode(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("workerName", p.client.workerName)
	req.Header.Set("X-Worker-Credential", p.credential)
	req.Header.Set("X-Assignment-Generation", strconv.FormatUint(claim.Generation, 10))
	req.Header.Set("X-Assignment-Capability", claim.Capability)
	req.Header.Set("X-Worker-Instance", claim.InstanceID.String())
	return req, nil
}

func protectedTransferStatus(resp *http.Response, want int) error {
	if resp.StatusCode == want {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusConflict:
		return ErrProtectedConflict
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return ErrProtectedUnavailable
	default:
		return fmt.Errorf("protected transfer failed: HTTP %d", resp.StatusCode)
	}
}

func (p *ProtectedClient) Download(ctx context.Context, claim model.AssignmentClaim) (*ProtectedDownload, error) {
	req, err := p.transferRequest(ctx, http.MethodGet, protectedDownloadPath, claim, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if err := protectedTransferStatus(resp, http.StatusOK); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	name := filepath.Base(params["filename"])
	if err != nil || name == "." || name == "" || resp.ContentLength < 0 {
		_ = resp.Body.Close()
		return nil, errors.New("server returned invalid protected download metadata")
	}
	return &ProtectedDownload{Body: resp.Body, Name: name, Size: resp.ContentLength}, nil
}

func (p *ProtectedClient) Checksum(ctx context.Context, claim model.AssignmentClaim) (string, error) {
	req, err := p.transferRequest(ctx, http.MethodGet, protectedChecksumPath, claim, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.client.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := protectedTransferStatus(resp, http.StatusOK); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65))
	if err != nil {
		return "", err
	}
	decoded, err := hex.DecodeString(string(data))
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("server returned an invalid protected checksum")
	}
	return hex.EncodeToString(decoded), nil
}

// Upload performs one attempt. After an uncertain response, the caller must
// reopen the source and retry the same assignment and checksum.
func (p *ProtectedClient) Upload(ctx context.Context, claim model.AssignmentClaim, source io.Reader, size int64, checksum string) error {
	decoded, err := hex.DecodeString(checksum)
	if source == nil || size < 0 || err != nil || len(decoded) != sha256.Size {
		return errors.New("invalid protected upload metadata")
	}
	req, err := p.transferRequest(ctx, http.MethodPost, protectedUploadPath, claim, source)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("checksum", hex.EncodeToString(decoded))
	resp, err := p.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return protectedTransferStatus(resp, http.StatusCreated)
}
