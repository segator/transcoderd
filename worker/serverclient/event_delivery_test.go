package serverclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/web"
)

func TestNotificationDelivery(t *testing.T) {
	t.Run("explicit expiry stops retries", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("X-TranscoderD-Error", "assignment-expired")
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()
		client := NewServerClient(&web.Config{Domain: server.URL}, "worker")
		err := client.publishEventContext(context.Background(), &model.EnvelopEvent{EventType: model.NotificationEvent})
		if !errors.Is(err, ErrAssignmentExpired) || requests.Load() != 1 {
			t.Fatalf("publish = %v, requests = %d", err, requests.Load())
		}
	})
	t.Run("ambiguous rejection does not discard work", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()
		client := NewServerClient(&web.Config{Domain: server.URL}, "worker")
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := client.publishEventContext(ctx, &model.EnvelopEvent{EventType: model.NotificationEvent})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("publish = %v, want retry until context cancellation", err)
		}
	})
	t.Run("transient server error is retried", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if requests.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		}))
		defer server.Close()
		client := NewServerClient(&web.Config{Domain: server.URL}, "worker")
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := client.publishEventContext(ctx, &model.EnvelopEvent{EventType: model.NotificationEvent}); err != nil {
			t.Fatal(err)
		}
		if requests.Load() != 2 {
			t.Fatalf("requests = %d, want 2", requests.Load())
		}
	})
}

func TestNotificationRetrySendsIdenticalPayloadAfterLostResponse(t *testing.T) {
	var first []byte
	var mu sync.Mutex
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if attempts.Add(1) == 1 {
			first = body
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		if string(body) != string(first) {
			t.Errorf("retry changed payload")
		}
	}))
	defer server.Close()
	client := NewServerClient(&web.Config{Domain: server.URL}, "worker")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.publishEventContext(ctx, &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: []byte(`{"eventID":4}`)}); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}
