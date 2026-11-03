// This executable is an isolated CI release feed. It is never distributed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"transcoder/update/managed"
)

type state struct {
	Enabled      bool   `json:"enabled"`
	BadSignature bool   `json:"badSignature"`
	BadDownload  bool   `json:"badDownload"`
	Version      string `json:"version"`
}

func main() {
	root := flag.String("root", "", "fixture directory")
	flag.Parse()
	if *root == "" {
		os.Exit(2)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var s state
		b, _ := os.ReadFile(filepath.Join(*root, "feed.json"))
		_ = json.Unmarshal(b, &s)
		if s.Version == "" {
			s.Version = "v99.1.0"
		}
		base := "http://127.0.0.1:18781"
		dir := filepath.Join(*root, s.Version)
		switch r.URL.Path {
		case "/latest":
			assets := []map[string]string{}
			if s.Enabled {
				for _, name := range []string{managed.ManifestName, managed.SignatureName, "transcoderd-worker-" + s.Version + "-windows-amd64-setup.exe"} {
					assets = append(assets, map[string]string{"name": name, "browser_download_url": base + "/" + name})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": s.Version, "draft": false, "prerelease": false, "assets": assets})
		case "/" + managed.SignatureName:
			if s.BadSignature {
				_, _ = fmt.Fprint(w, "invalid signature")
				return
			}
			http.ServeFile(w, r, filepath.Join(dir, managed.SignatureName))
		case "/" + managed.ManifestName:
			http.ServeFile(w, r, filepath.Join(dir, managed.ManifestName))
		default:
			if r.URL.Path != "/transcoderd-worker-"+s.Version+"-windows-amd64-setup.exe" {
				http.NotFound(w, r)
				return
			}
			for {
				if _, err := os.Stat(filepath.Join(*root, "hold-download")); os.IsNotExist(err) {
					break
				}
				select {
				case <-r.Context().Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			if s.BadDownload {
				_, _ = fmt.Fprint(w, "corrupt installer")
				return
			}
			// The request selects an allowlisted asset; never form a disk path
			// from the request URL, even in the isolated fixture server.
			http.ServeFile(w, r, filepath.Join(dir, "transcoderd-worker-"+s.Version+"-windows-amd64-setup.exe"))
		}
	})
	srv := &http.Server{Addr: "127.0.0.1:18781", Handler: h, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
