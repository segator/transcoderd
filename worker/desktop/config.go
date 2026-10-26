package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	workerconfig "transcoder/worker/config"
)

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type Settings struct {
	Domain   string   `json:"domain"`
	Token    string   `json:"token,omitempty"`
	Name     string   `json:"name"`
	Codec    string   `json:"codec"`
	MaxJobs  int      `json:"maxJobs"`
	Schedule []string `json:"schedule"`
}

type PublicSettings struct {
	Domain   string   `json:"domain"`
	TokenSet bool     `json:"tokenSet"`
	Name     string   `json:"name"`
	Codec    string   `json:"codec"`
	MaxJobs  int      `json:"maxJobs"`
	Schedule []string `json:"schedule"`
}

func (s Settings) Public() PublicSettings {
	return PublicSettings{Domain: s.Domain, TokenSet: s.Token != "", Name: s.Name, Codec: s.Codec, MaxJobs: s.MaxJobs, Schedule: s.Schedule}
}

func (s Settings) Validate() error {
	u, err := url.Parse(s.Domain)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("server URL must be an HTTP(S) URL without credentials, query or fragment")
	}
	if strings.TrimSpace(s.Token) == "" || len(s.Token) > 4096 || strings.ContainsAny(s.Token, "\x00\r\n") {
		return errors.New("a valid server token is required")
	}
	if !validName.MatchString(s.Name) {
		return errors.New("worker name must contain only letters, numbers, dot, dash or underscore")
	}
	if s.MaxJobs < 0 || s.MaxJobs > 64 {
		return errors.New("concurrent encodes must be between 0 and 64")
	}
	if len(s.Schedule) > 32 {
		return errors.New("too many schedule windows")
	}
	c := &workerconfig.Config{MaxActiveJobs: s.MaxJobs, EncodeConfig: &workerconfig.FFMPEGConfig{VideoCodec: s.Codec}, Schedule: &workerconfig.ScheduleConfig{TimeZone: "Local", Windows: s.Schedule}}
	return c.Validate()
}

type Persisted struct {
	Active       *Settings `json:"active,omitempty"`
	Pending      *Settings `json:"pending,omitempty"`
	PendingLimit bool      `json:"pendingLimit,omitempty"`
}

func Load(dir string) (Persisted, error) {
	var state Persisted
	data, err := os.ReadFile(filepath.Join(dir, "service.json"))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("read saved settings: %w", err)
	}
	if state.Active != nil {
		if err = state.Active.Validate(); err != nil {
			return state, fmt.Errorf("invalid saved settings: %w", err)
		}
	}
	if state.Pending != nil {
		if err = state.Pending.Validate(); err != nil {
			return state, fmt.Errorf("invalid pending settings: %w", err)
		}
	}
	return state, nil
}

// Save writes to a temporary file in the already-protected service directory.
func Save(dir string, state Persisted) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(dir, "service.json", data)
}

func writeAtomic(dir, filename string, data []byte) error {
	f, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filepath.Join(dir, filename))
}

// WorkerConfig is JSON, which Viper accepts as YAML, and never puts the token in argv.
func WorkerConfig(dir string, s Settings) error {
	value := map[string]any{"web": map[string]any{"domain": s.Domain, "token": s.Token}, "worker": map[string]any{"name": s.Name, "maxActiveJobs": s.MaxJobs, "temporalPath": filepath.Join(dir, "work"), "ffmpegConfig": map[string]any{"videoCodec": s.Codec}, "schedule": map[string]any{"timeZone": "Local", "windows": s.Schedule}}}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeAtomic(dir, "config.yaml", data)
}
