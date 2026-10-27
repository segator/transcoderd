// Package managed implements complete, authenticated Windows installation updates.
package managed

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
)

const ManifestName = "windows-update.json"
const SignatureName = ManifestName + ".sig"
const MaxManifestSize = 4 << 20
const MaxInstallerSize int64 = 4 << 30

//go:embed release-key.pub
var releaseKey string

// FixtureKey and FixtureEndpoint are set only in isolated CI fixture binaries.
// Production builds never read endpoint/key overrides from environment or IPC.
var FixtureKey, FixtureEndpoint, FixtureMode string

type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Schema       int    `json:"schema"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	Installer    File   `json:"installer"`
	Files        []File `json:"files"`
}

func PublicKey() (ed25519.PublicKey, error) {
	key := releaseKey
	if FixtureMode == "true" {
		key = FixtureKey
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(data) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Windows update verification key")
	}
	return ed25519.PublicKey(data), nil
}

func Version(value string) (semver.Version, error) {
	return semver.Parse(strings.TrimPrefix(value, "v"))
}

func Verify(data, signature []byte, key ed25519.PublicKey) (*Manifest, error) {
	if len(data) > MaxManifestSize || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, data, signature) {
		return nil, errors.New("windows update signature verification failed")
	}
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("read Windows update manifest: %w", err)
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("unexpected Windows update manifest data")
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m Manifest) Validate() error {
	v, err := Version(m.Version)
	if err != nil || len(v.Pre) != 0 || m.Schema != 1 || m.Architecture != "amd64" {
		return errors.New("unsupported Windows update release")
	}
	expected := "transcoderd-worker-" + m.Version + "-windows-amd64-setup.exe"
	if m.Installer.Name != expected || m.Installer.Size <= 0 || m.Installer.Size > MaxInstallerSize || !validDigest(m.Installer.SHA256) {
		return errors.New("invalid Windows update installer identity")
	}
	if len(m.Files) == 0 || len(m.Files) > 20000 {
		return errors.New("invalid Windows update file list")
	}
	seen := make(map[string]bool)
	for _, f := range m.Files {
		name := strings.ToLower(f.Name)
		if !SafeName(f.Name) || seen[name] || f.Size < 0 || f.Size > 8<<30 || !validDigest(f.SHA256) {
			return fmt.Errorf("invalid Windows update file %q", f.Name)
		}
		seen[name] = true
	}
	for _, required := range []string{"transcoderd-worker.exe", "transcoderd-desktop-service.exe", "transcoderd-updater.exe", "transcoderd-tray.exe", "wwwroot/index.html"} {
		if !seen[required] {
			return fmt.Errorf("windows update is missing %s", required)
		}
	}
	return nil
}

func validDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size
}

func SafeName(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\:\x00<>\"|?*") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "NUL" || base == "PRN" || base == "AUX" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return false
		}
	}
	return true
}

func DescribeFile(path, name string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return File{Name: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, err
}

func VerifyFile(path string, expected File) error {
	if err := CheckPath(path); err != nil {
		return err
	}
	got, err := DescribeFile(path, expected.Name)
	if err != nil {
		return err
	}
	if got.Size != expected.Size || !strings.EqualFold(got.SHA256, expected.SHA256) {
		return fmt.Errorf("windows update file verification failed: %s", expected.Name)
	}
	return nil
}

func InstallationFiles(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := CheckPath(path); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if !SafeName(name) {
			return fmt.Errorf("invalid installation file %s", name)
		}
		f, err := DescribeFile(path, name)
		if err != nil {
			return err
		}
		files = append(files, f)
		return nil
	})
	return files, err
}

func VerifyInstallation(root string, files []File) error {
	for _, f := range files {
		if !SafeName(f.Name) {
			return errors.New("unsafe installation file")
		}
		if err := VerifyFile(filepath.Join(root, filepath.FromSlash(f.Name)), f); err != nil {
			return err
		}
	}
	return nil
}
