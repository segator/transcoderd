package managed

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ReleaseEndpoint = "https://api.github.com/repos/segator/transcoderd/releases/latest"

type Client struct {
	HTTP     *http.Client
	Endpoint string
	Key      ed25519.PublicKey
	Fixture  bool
}
type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}
type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func NewClient() (*Client, error) {
	key, err := PublicKey()
	if err != nil {
		return nil, err
	}
	endpoint := ReleaseEndpoint
	fixture := FixtureMode == "true"
	if fixture {
		endpoint = FixtureEndpoint
	}
	c := &Client{Endpoint: endpoint, Key: key, Fixture: fixture}
	c.HTTP = &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many update redirects")
		}
		return c.validateURL(req.URL.String())
	}}
	return c, nil
}

func (c *Client) validateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u == nil || u.User != nil {
		return errors.New("invalid Windows update URL")
	}
	if c.Fixture {
		base, e := url.Parse(c.Endpoint)
		if e == nil && u.Host == base.Host && (u.Scheme == "http" || u.Scheme == "https") {
			return nil
		}
	}
	if u.Scheme != "https" || u.Port() != "" {
		return errors.New("windows updates require HTTPS")
	}
	host := strings.ToLower(u.Hostname())
	if host == "api.github.com" || host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com") {
		return nil
	}
	return errors.New("windows update URL is not a GitHub release host")
}

func (c *Client) get(ctx context.Context, value string) (*http.Response, error) {
	if err := c.validateURL(value); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TranscoderD-Windows-Updater")
	req.Header.Set("Accept", "application/octet-stream")
	if strings.HasPrefix(value, "https://api.github.com/") {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	r, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		r.Body.Close()
		return nil, fmt.Errorf("windows update request returned HTTP %d", r.StatusCode)
	}
	return r, nil
}
func (c *Client) read(ctx context.Context, value string, max int64) ([]byte, error) {
	r, err := c.get(ctx, value)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if int64(len(b)) > max {
		return nil, errors.New("windows update metadata exceeds size limit")
	}
	return b, err
}

func (c *Client) Check(ctx context.Context, current string) (*Candidate, *Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, err := c.read(ctx, c.Endpoint, MaxManifestSize)
	if err != nil {
		return nil, nil, err
	}
	var r release
	if err = json.Unmarshal(b, &r); err != nil {
		return nil, nil, err
	}
	v, err := Version(r.Tag)
	if err != nil {
		return nil, nil, err
	}
	installed, err := Version(current)
	if err != nil {
		return nil, nil, err
	}
	if r.Draft || r.Prerelease || len(v.Pre) > 0 || !v.GT(installed) {
		return nil, nil, nil
	}
	urls := make(map[string]string)
	for _, a := range r.Assets {
		if _, ok := urls[a.Name]; ok {
			return nil, nil, errors.New("duplicate release asset")
		}
		urls[a.Name] = a.URL
	}
	if urls[ManifestName] == "" || urls[SignatureName] == "" {
		return nil, nil, nil
	}
	data, err := c.read(ctx, urls[ManifestName], MaxManifestSize)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := c.read(ctx, urls[SignatureName], 1024)
	if err != nil {
		return nil, nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return nil, nil, err
	}
	m, err := Verify(data, sig, c.Key)
	if err != nil {
		return nil, nil, err
	}
	if m.Version != r.Tag {
		return nil, nil, errors.New("windows update tag and manifest do not match")
	}
	if urls[m.Installer.Name] == "" {
		return nil, nil, nil
	}
	if err = c.validateURL(urls[m.Installer.Name]); err != nil {
		return nil, nil, err
	}
	return &Candidate{Data: data, Signature: sig, URL: urls[m.Installer.Name]}, m, nil
}

type progressWriter struct {
	file   *os.File
	n      int64
	report func(int64)
}

func (w *progressWriter) Write(b []byte) (int, error) {
	n, err := w.file.Write(b)
	w.n += int64(n)
	if w.report != nil {
		w.report(w.n)
	}
	return n, err
}

func (c *Client) Stage(ctx context.Context, dir string, candidate *Candidate, report func(int64)) (string, error) {
	m, err := Verify(candidate.Data, candidate.Signature, c.Key)
	if err != nil {
		return "", err
	}
	stage := StageDir(dir, m)
	if err = ProtectDir(stage); err != nil {
		return "", err
	}
	path := filepath.Join(stage, m.Installer.Name)
	if VerifyFile(path, m.Installer) == nil {
		if report != nil {
			report(m.Installer.Size)
		}
		return path, nil
	}
	required := m.Installer.Size
	for _, f := range m.Files {
		required += 2 * f.Size
	}
	if err = AvailableSpace(stage, required); err != nil {
		return "", err
	}
	if err = CheckPath(path); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(stage, "installer-*.part")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	r, err := c.get(ctx, candidate.URL)
	if err != nil {
		f.Close()
		return "", err
	}
	defer r.Body.Close()
	h := sha256.New()
	w := &progressWriter{file: f, report: report}
	n, copyErr := io.Copy(io.MultiWriter(w, h), io.LimitReader(r.Body, m.Installer.Size+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if syncErr != nil {
		return "", syncErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n != m.Installer.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.Installer.SHA256) {
		return "", errors.New("downloaded Windows installer failed integrity verification")
	}
	if err = protectFile(tmp); err != nil {
		return "", err
	}
	if err = ReplaceFile(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func StageDir(dir string, m *Manifest) string {
	return filepath.Join(dir, "updates", m.Installer.SHA256)
}
