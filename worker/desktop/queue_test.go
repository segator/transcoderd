package desktop

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchQueue(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		body        string
		unsupported bool
	}{{"success", 200, `{"total":1,"jobs":[{"id":"id","name":"movie.mkv"}]}`, false}, {"old server", 404, ``, true}, {"server error", 500, ``, false}} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer token" || r.URL.Path != "/api/v1/jobs/queued" {
					t.Errorf("invalid request")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			q, err := FetchQueue(context.Background(), Settings{Domain: srv.URL, Token: "token"})
			if tt.status == 200 {
				if err != nil || q.Total != 1 || q.Jobs[0].Name != "movie.mkv" {
					t.Fatalf("queue = %+v, error = %v", q, err)
				}
			} else if err == nil || errors.Is(err, ErrQueueUnsupported) != tt.unsupported {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
