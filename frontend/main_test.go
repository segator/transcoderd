package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendAuthEnvironmentAndCLI(t *testing.T) {
	file := filepath.Join(t.TempDir(), "frontend.yaml")
	if err := os.WriteFile(file, []byte("frontend:\n  auth:\n    mode: token\n    oidc:\n      clientID: file-client\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"TR_FRONTEND_AUTH_MODE": "oidc", "TR_FRONTEND_API_TOKEN": "private-api-credential",
		"TR_FRONTEND_AUTH_SESSION_KEY": "session-key", "TR_FRONTEND_AUTH_OIDC_ISSUER_URL": "https://auth.example/application/o/transcoderd/",
		"TR_FRONTEND_AUTH_OIDC_CLIENT_ID": "env-client", "TR_FRONTEND_AUTH_OIDC_CLIENT_SECRET": "private-client-credential",
		"TR_FRONTEND_AUTH_OIDC_SCOPES": "openid,profile,groups", "TR_FRONTEND_AUTH_OIDC_ALLOWED_GROUPS": "operators,admins",
	} {
		t.Setenv(key, value)
	}
	for _, mode := range []string{"oidc", "token"} {
		args := []string{"--config", file}
		if mode == "token" {
			args = append(args, "--frontend.auth.mode=token", "--frontend.auth.oidc.clientID=cli-client")
		}
		opts, err := readOptions(args)
		if err != nil {
			t.Fatal(err)
		}
		wantClient := "env-client"
		if mode == "token" {
			wantClient = "cli-client"
		}
		if opts.Frontend.Auth.Mode != mode || opts.Frontend.Auth.OIDC.ClientID != wantClient || opts.Frontend.APIToken != "private-api-credential" || opts.Frontend.Auth.SessionKey != "session-key" || opts.Frontend.Auth.OIDC.ClientSecret != "private-client-credential" || opts.Frontend.Auth.OIDC.IssuerURL != "https://auth.example/application/o/transcoderd/" || len(opts.Frontend.Auth.OIDC.Scopes) != 3 || len(opts.Frontend.Auth.OIDC.AllowedGroups) != 2 {
			t.Error("readOptions() did not apply auth environment bindings and CLI precedence")
		}
	}
}

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
