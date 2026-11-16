package serverclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/web"

	"github.com/google/uuid"
)

func TestProtectedClientUsesDurableRequestIdentityAndLeaseDeadline(t *testing.T) {
	jobID, requestID, instance := uuid.New(), uuid.New(), uuid.New()
	credential := "worker-specific-secret"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("workerName") != "node" || r.Header.Get("X-Worker-Credential") != credential || strings.Contains(r.URL.String(), credential) {
			t.Error("protected identity or credential was not sent safely")
		}
		switch r.URL.Path {
		case protectedRequestPath:
			var request struct {
				RequestID string `json:"requestId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.RequestID != requestID.String() {
				t.Errorf("request identity = %+v, %v", request, err)
			}
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: jobID}, Generation: 2, KeyID: "v1", Capability: "capability"})
		case protectedLeasePath:
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"leaseSeconds":60}`))
		case protectedParkPath:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	base := NewServerClient(&web.Config{Domain: server.URL}, "node")
	client, err := base.Protected(credential)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RequestJob(t.Context(), requestID); err == nil || strings.Contains(err.Error(), credential) {
		t.Fatalf("first request = %v, want safe error", err)
	}
	assignment, err := client.RequestJob(t.Context(), requestID)
	if err != nil || assignment.Id != jobID || calls != 2 {
		t.Fatalf("retried request = %+v, %v; calls = %d", assignment, err, calls)
	}
	claim := model.AssignmentClaim{JobID: jobID, Generation: 2, Capability: "capability", InstanceID: instance}
	deadline, err := client.AcquireLease(context.Background(), claim)
	now := time.Now()
	if err != nil || deadline.After(now.Add(time.Minute-10*time.Millisecond)) || !deadline.After(now) {
		t.Fatalf("lease deadline = %v, %v", deadline, err)
	}
	if err := client.Park(t.Context(), claim); err != nil {
		t.Fatalf("Park() = %v", err)
	}
}

func TestProtectedTransfersUseClaimAndValidateMetadata(t *testing.T) {
	claim := model.AssignmentClaim{JobID: uuid.New(), Generation: 3, Capability: "assignment-secret", InstanceID: uuid.New()}
	data := []byte("movie data")
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("uuid") != claim.JobID.String() || r.Header.Get("X-Worker-Instance") != claim.InstanceID.String() || r.Header.Get("X-Assignment-Generation") != "3" || r.Header.Get("X-Assignment-Capability") != claim.Capability || r.Header.Get("X-Worker-Credential") != "worker-secret" || strings.Contains(r.URL.String(), claim.Capability) {
			t.Error("protected transfer omitted its claim or put a secret in the URL")
		}
		switch r.URL.Path {
		case protectedDownloadPath:
			w.Header().Set("Content-Disposition", `attachment; filename="movie.mkv"`)
			_, _ = w.Write(data)
		case protectedChecksumPath:
			_, _ = w.Write([]byte(checksum))
		case protectedUploadPath:
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != string(data) || r.Header.Get("checksum") != checksum || r.ContentLength != int64(len(data)) {
				t.Errorf("upload = %q, length=%d, error=%v", body, r.ContentLength, err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	base := NewServerClient(&web.Config{Domain: server.URL}, "node")
	client, err := base.Protected("worker-secret")
	if err != nil {
		t.Fatal(err)
	}
	download, err := client.Download(t.Context(), claim)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if err != nil || string(body) != string(data) || download.Name != "movie.mkv" || download.Size != int64(len(data)) {
		t.Fatalf("download = %+v %q, %v", download, body, err)
	}
	if got, err := client.Checksum(t.Context(), claim); err != nil || got != checksum {
		t.Fatalf("checksum = %q, %v", got, err)
	}
	if err := client.Upload(t.Context(), claim, strings.NewReader(string(data)), int64(len(data)), checksum); err != nil {
		t.Fatalf("Upload() = %v", err)
	}
}

func TestProtectedClientFailsClosedWithoutProtocol(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	base := NewServerClient(&web.Config{Domain: server.URL}, "node")
	client, err := base.Protected("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RequestJob(t.Context(), uuid.New()); !errors.Is(err, ErrProtectedUnavailable) {
		t.Fatalf("RequestJob() = %v, want unavailable", err)
	}
}
