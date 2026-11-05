package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Reset during ClientHello to reproduce curl's TLS error 35 from main CI.
type resetTLSListener struct {
	net.Listener
	resets   int32
	accepted atomic.Int32
}

func (l *resetTLSListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.accepted.Add(1) > l.resets {
			return conn, nil
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var hello [1]byte
		_, _ = conn.Read(hello[:])
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetLinger(0)
		}
		_ = conn.Close()
	}
}

func TestWindowsSourceDownload(t *testing.T) {
	for _, tool := range []string{"bash", "curl", "tar", "sha256sum", "mktemp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("source download test requires %s: %v", tool, err)
		}
	}
	archive := sourceTestArchive(t)
	checksum := fmt.Sprintf("%x", sha256.Sum256(archive))
	helper, err := os.ReadFile(filepath.Join(projectRoot(), "scripts", "windows", "fetch.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		resets       int32
		partial      bool
		badChecksum  bool
		wantError    string
		wantRequests int32
		wantAccepted int32
	}{
		{name: "TLS reset then success", resets: 1, wantRequests: 1, wantAccepted: 2},
		{name: "partial transfer then success", partial: true, wantRequests: 2},
		{name: "checksum mismatch", badChecksum: true, wantError: "FAILED", wantRequests: 1},
		{name: "TLS retries exhausted", resets: 100, wantError: "curl: (35)", wantAccepted: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 && tt.partial {
					w.Header().Set("Content-Length", fmt.Sprint(len(archive)))
					_, _ = w.Write(archive[:len(archive)/2])
					return
				}
				_, _ = w.Write(archive)
			}))
			listener := &resetTLSListener{Listener: server.Listener, resets: tt.resets}
			server.Listener = listener
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.StartTLS()
			defer server.Close()

			dir := t.TempDir()
			ca := filepath.Join(dir, "ca.pem")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
				t.Fatal(err)
			}
			digest := checksum
			if tt.badChecksum {
				digest = strings.Repeat("0", 64)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-c", string(helper)+"\n"+`fetch source "$1" "$2"`, "test-fetch", server.URL, digest)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "CURL_CA_BUNDLE="+ca, "CURL_HOME="+dir, "NO_PROXY=*", "no_proxy=*")
			output, err := cmd.CombinedOutput()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("fetch() = %v, want success:\n%s", err, output)
				}
				if tt.resets > 0 && !bytes.Contains(output, []byte("curl: (35)")) {
					t.Fatalf("fetch() did not exercise TLS error 35:\n%s", output)
				}
				if tt.partial && !bytes.Contains(output, []byte("curl: (18)")) {
					t.Fatalf("fetch() did not exercise partial transfer error 18:\n%s", output)
				}
				got, err := os.ReadFile(filepath.Join(dir, "source", "file.txt"))
				if err != nil || string(got) != "verified source\n" {
					t.Fatalf("fetch() extracted %q, %v, want verified source", got, err)
				}
			} else {
				if err == nil || !bytes.Contains(output, []byte(tt.wantError)) {
					t.Fatalf("fetch() = %v, want %q:\n%s", err, tt.wantError, output)
				}
				if _, err := os.Stat(filepath.Join(dir, "source")); !os.IsNotExist(err) {
					t.Fatalf("fetch() created source directory on failure: %v", err)
				}
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Errorf("fetch() requests = %d, want %d", got, tt.wantRequests)
			}
			if tt.wantAccepted != 0 && listener.accepted.Load() != tt.wantAccepted {
				t.Errorf("fetch() connections = %d, want %d", listener.accepted.Load(), tt.wantAccepted)
			}
			if files, err := filepath.Glob(filepath.Join(dir, "source.tar.gz.*")); err != nil || len(files) != 0 {
				t.Errorf("fetch() temporary archives = %v, %v, want none", files, err)
			}
		})
	}
}

func sourceTestArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	writer := tar.NewWriter(zipped)
	content := "verified source\n"
	if err := writer.WriteHeader(&tar.Header{Name: "upstream/file.txt", Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
