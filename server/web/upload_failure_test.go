package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failedUploadSink struct {
	checksum string
	short    bool
	cleaned  bool
}

func (s *failedUploadSink) Write(data []byte) (int, error) {
	if s.short {
		return len(data) - 1, nil
	}
	return 0, errors.New("disk full")
}
func (s *failedUploadSink) Close(bool) error { return nil }
func (s *failedUploadSink) Clean() error     { s.cleaned = true; return nil }
func (s *failedUploadSink) GetHash() string  { return s.checksum }

func TestReceiveUploadNeverCommitsFailedOrShortWrite(t *testing.T) {
	for _, short := range []bool{false, true} {
		sum := sha256.Sum256([]byte("encoded"))
		sink := &failedUploadSink{checksum: hex.EncodeToString(sum[:]), short: short}
		request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("encoded"))
		request.Header.Set("Content-Length", "7")
		request.Header.Set("checksum", sink.checksum)
		response := httptest.NewRecorder()
		committed := false
		receiveUpload(response, request, sink, func(string, int64) error { committed = true; return nil })
		if response.Code != http.StatusInternalServerError || committed || !sink.cleaned {
			t.Fatalf("short=%t: status=%d committed=%t cleaned=%t", short, response.Code, committed, sink.cleaned)
		}
	}
}
