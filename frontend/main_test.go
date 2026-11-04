package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendConfigurationPrecedence(t *testing.T) {
	config := filepath.Join(t.TempDir(), "frontend.yaml")
	if err := os.WriteFile(config, []byte("frontend:\n  listen: 127.0.0.1:8181\n  apiURL: https://api.file.test\n  publicURL: https://ui.file.test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, apiEnv, listenEnv, publicEnv string
		flags                              []string
		wantAPI, wantListen, wantPublic    string
	}{
		{"file", "", "", "", nil, "https://api.file.test", "127.0.0.1:8181", "https://ui.file.test"},
		{"environment", "http://api.env.test:8080", ":9191", "https://ui.env.test", nil, "http://api.env.test:8080", ":9191", "https://ui.env.test"},
		{"CLI", "http://api.env.test", ":9191", "https://ui.env.test", []string{"--frontend.apiURL=http://api.flag.test", "--frontend.listen=:9292", "--frontend.publicURL=https://ui.flag.test"}, "http://api.flag.test", ":9292", "https://ui.flag.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TR_FRONTEND_API_URL", tt.apiEnv)
			t.Setenv("TR_FRONTEND_LISTEN", tt.listenEnv)
			t.Setenv("TR_FRONTEND_PUBLIC_URL", tt.publicEnv)
			opts, err := readOptions(append([]string{"--config", config}, tt.flags...))
			if err != nil || opts.Frontend.APIURL != tt.wantAPI || opts.Frontend.Listen != tt.wantListen || opts.Frontend.PublicURL != tt.wantPublic {
				t.Errorf("readOptions() = %+v, %v", opts, err)
			}
		})
	}
}

func TestFrontendConfigErrorsAndVersion(t *testing.T) {
	config := filepath.Join(t.TempDir(), "frontend.yaml")
	if err := os.WriteFile(config, []byte("frontend: [broken"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--config", config}, {"--config", config + ".missing"}, {"--unknown"}, {"extra-positional"}} {
		if _, err := readOptions(args); err == nil {
			t.Errorf("readOptions(%v) accepted invalid configuration", args)
		}
	}
	// No API connection, database or filesystem library is needed for version.
	if err := run(t.Context(), []string{"--version", "--frontend.apiURL=http://127.0.0.1:1"}); err != nil {
		t.Errorf("run(--version) = %v", err)
	}
}
