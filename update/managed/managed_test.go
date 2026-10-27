package managed

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureManifest(t *testing.T) (Manifest, ed25519.PublicKey, ed25519.PrivateKey, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	installer := []byte("test installer bytes")
	digest := sha256.Sum256(installer)
	m := Manifest{Schema: 1, Version: "v99.1.0", Architecture: "amd64", Installer: File{Name: "transcoderd-worker-v99.1.0-windows-amd64-setup.exe", Size: int64(len(installer)), SHA256: hex.EncodeToString(digest[:])}}
	for _, name := range []string{"transcoderd-worker.exe", "transcoderd-desktop-service.exe", "transcoderd-updater.exe", "transcoderd-tray.exe", "wwwroot/index.html"} {
		m.Files = append(m.Files, File{Name: name, Size: 1, SHA256: strings.Repeat("0", 64)})
	}
	return m, pub, key, installer
}

func TestManifestValidation(t *testing.T) {
	m, pub, key, _ := fixtureManifest(t)
	tests := []struct {
		name string
		edit func(*Manifest)
	}{
		{"architecture", func(m *Manifest) { m.Architecture = "arm64" }},
		{"prerelease", func(m *Manifest) { m.Version = "v99.1.0-rc1" }},
		{"wrong installer", func(m *Manifest) { m.Installer.Name = "other.exe" }},
		{"huge installer", func(m *Manifest) { m.Installer.Size = MaxInstallerSize + 1 }},
		{"missing worker", func(m *Manifest) { m.Files = m.Files[1:] }},
		{"duplicate case insensitive", func(m *Manifest) {
			m.Files = append(m.Files, m.Files[0])
			m.Files[len(m.Files)-1].Name = "TRANSCODERD-WORKER.exe"
		}},
		{"traversal", func(m *Manifest) { m.Files[0].Name = "../worker.exe" }},
		{"windows path", func(m *Manifest) { m.Files[0].Name = "C:\\worker.exe" }},
		{"device", func(m *Manifest) { m.Files[0].Name = "con.exe" }},
	}
	data, _ := json.Marshal(m)
	if _, err := Verify(data, ed25519.Sign(key, data), pub); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(append(data, ' '), ed25519.Sign(key, data), pub); err == nil {
		t.Fatal("Verify() accepted changed signed bytes")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := m
			copy.Files = append([]File(nil), m.Files...)
			tt.edit(&copy)
			data, _ := json.Marshal(copy)
			if _, err := Verify(data, ed25519.Sign(key, data), pub); err == nil {
				t.Fatal("Verify() accepted invalid manifest")
			}
		})
	}
}

func fixtureClient(t *testing.T, installerHandler http.HandlerFunc) *Client {
	t.Helper()
	m, pub, key, _ := fixtureManifest(t)
	return fixtureRelease(t, m, pub, key, installerHandler)
}

func fixtureRelease(t *testing.T, m Manifest, pub ed25519.PublicKey, key ed25519.PrivateKey, installerHandler http.HandlerFunc) *Client {
	t.Helper()
	installer := []byte("test installer bytes")
	data, _ := json.Marshal(m)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, data))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(release{Tag: m.Version, Assets: []asset{{ManifestName, server.URL + "/manifest"}, {SignatureName, server.URL + "/signature"}, {m.Installer.Name, server.URL + "/installer"}}})
		case "/manifest":
			_, _ = w.Write(data)
		case "/signature":
			_, _ = fmt.Fprint(w, sig)
		case "/installer":
			if installerHandler != nil {
				installerHandler(w, r)
			} else {
				_, _ = w.Write(installer)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return &Client{HTTP: server.Client(), Endpoint: server.URL + "/latest", Key: pub, Fixture: true}
}

func TestMandatoryUpdateBlocksBeforeDownloadAndDrains(t *testing.T) {
	started := make(chan struct{})
	releaseDownload := make(chan struct{})
	c := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-releaseDownload
		_, _ = w.Write([]byte("test installer bytes"))
	})
	dir := t.TempDir()
	active := 1
	installed := 0
	m := NewManager(dir, "v2.8.1", c, Hooks{Drain: func() (int, int, error) {
		state, err := Load(dir)
		if err != nil || !state.Pending {
			t.Error("drain started without persisted pending state")
		}
		return active, 0, nil
	}, Install: func(_ *Candidate, _ *Manifest, _ string) error { installed++; return nil }})
	done := make(chan error, 1)
	go func() { done <- m.Step(context.Background(), time.Now(), true) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	s, err := Load(dir)
	if err != nil || !s.Pending || s.ConfigurationAllowed("configure") || s.ConfigurationAllowed("set-max-jobs") {
		t.Fatalf("pending state did not block configuration: %+v %v", s, err)
	}
	if !m.Snapshot().Pending {
		t.Error("admission was not blocked before download")
	}
	close(releaseDownload)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if installed != 0 || m.Snapshot().Phase != "draining" {
		t.Fatal("installer ran with accepted work")
	}
	active = 0
	if err = m.Step(context.Background(), time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if installed != 1 {
		t.Errorf("installer calls = %d, want 1", installed)
	}
}

func TestPendingSurvivesFailureAndRestart(t *testing.T) {
	c := fixtureClient(t, nil)
	dir := t.TempDir()
	now := time.Now()
	m := NewManager(dir, "v2.8.1", c, Hooks{Drain: func() (int, int, error) { return 0, 0, errors.New("worker control unavailable") }})
	if err := m.Step(context.Background(), now, true); err == nil {
		t.Fatal("expected control failure")
	}
	s, err := Load(dir)
	if err != nil || !s.Pending || s.NextRetry.Sub(now) != 15*time.Minute {
		t.Fatalf("retry state = %+v, %v", s, err)
	}
	restarted := NewManager(dir, "v2.8.1", c, Hooks{})
	if !restarted.Snapshot().Pending {
		t.Fatal("restart cleared admission barrier")
	}
	if RetryDelay(2) != time.Hour || RetryDelay(30) != 6*time.Hour {
		t.Fatal("retry delay is not bounded")
	}
}

func TestReleaseSelectionDoesNotApplyUnsupportedVersions(t *testing.T) {
	for _, tt := range []struct {
		name, tag         string
		draft, prerelease bool
	}{
		{"older", "v1.0.0", false, false}, {"same", "v2.8.1", false, false}, {"draft", "v99.1.0", true, false}, {"prerelease flag", "v99.1.0", false, true}, {"prerelease version", "v99.1.0-rc1", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(release{Tag: tt.tag, Draft: tt.draft, Prerelease: tt.prerelease})
			}))
			defer server.Close()
			c := &Client{Endpoint: server.URL, HTTP: server.Client(), Fixture: true}
			candidate, _, err := c.Check(context.Background(), "v2.8.1")
			if err != nil || candidate != nil {
				t.Fatalf("Check() = %v, %v, want no update", candidate, err)
			}
		})
	}
}

func TestRestoreTreeRemovesFailedReleaseFiles(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	install := filepath.Join(root, "install")
	if err := os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "app.exe"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := InstallationFiles(old)
	if err != nil {
		t.Fatal(err)
	}
	if err = CopyTree(old, install); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(install, "bad.dll"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = RestoreTree(old, install, files); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(install, "bad.dll")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback kept new version file")
	}
}

func TestNewerTargetSupersedesFailedUpdate(t *testing.T) {
	m, pub, key, _ := fixtureManifest(t)
	c := fixtureRelease(t, m, pub, key, nil)
	dir := t.TempDir()
	now := time.Now()
	manager := NewManager(dir, "v2.8.1", c, Hooks{Drain: func() (int, int, error) { return 0, 0, errors.New("worker unavailable") }})
	if err := manager.Step(context.Background(), now, true); err == nil {
		t.Fatal("expected pending retry")
	}
	m.Version = "v99.2.0"
	m.Installer.Name = "transcoderd-worker-v99.2.0-windows-amd64-setup.exe"
	manager.client = fixtureRelease(t, m, pub, key, nil)
	if err := manager.Step(context.Background(), now.Add(16*time.Minute), false); err == nil {
		t.Fatal("expected worker failure")
	}
	status := manager.Snapshot()
	if !status.Pending || status.TargetVersion != "v99.2.0" || status.Attempts != 1 {
		t.Fatalf("superseded state = %+v", status)
	}
}

func TestStageRejectsCorruptDownloadAndStorageFailure(t *testing.T) {
	for _, tt := range []struct {
		name    string
		storage bool
	}{{"corrupt installer", false}, {"unavailable staging directory", true}} {
		t.Run(tt.name, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("corrupt installer")) })
			candidate, _, err := c.Check(context.Background(), "v2.8.1")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if tt.storage {
				if err = os.WriteFile(filepath.Join(dir, "updates"), []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = c.Stage(context.Background(), dir, candidate, nil); err == nil {
				t.Fatal("Stage() accepted invalid download/storage")
			}
		})
	}
}

func TestStageRevalidatesCachedInstaller(t *testing.T) {
	c := fixtureClient(t, nil)
	candidate, manifest, err := c.Check(context.Background(), "v2.8.1")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, err := c.Stage(context.Background(), dir, candidate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Stage(context.Background(), dir, candidate, nil); err != nil {
		t.Fatal(err)
	}
	if err = VerifyFile(path, manifest.Installer); err != nil {
		t.Fatalf("cached installer was not repaired: %v", err)
	}
}

func TestUpdatePathsRejectSymlinkAncestors(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := ProtectDir(filepath.Join(link, "updates")); err == nil {
		t.Fatal("ProtectDir() followed an untrusted link")
	}
}

func TestMissingAssetsDoNotEstablishPendingTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(release{Tag: "v99.1.0"}) }))
	defer server.Close()
	c := &Client{Endpoint: server.URL, HTTP: server.Client(), Fixture: true}
	manager := NewManager(t.TempDir(), "v2.8.1", c, Hooks{})
	if err := manager.Step(context.Background(), time.Now(), true); err != nil {
		t.Fatal(err)
	}
	if manager.Snapshot().Pending {
		t.Fatal("incomplete release blocked admission")
	}
}
