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
	Domain   string `json:"domain"`
	Token    string `json:"token,omitempty"`
	Name     string `json:"name"`
	CPUCodec string `json:"cpuCodec,omitempty"`
	CPUJobs  int    `json:"cpuJobs"`
	GPUCodec string `json:"gpuCodec,omitempty"`
	GPUJobs  int    `json:"gpuJobs"`
	// Codec and MaxJobs keep older service.json files and local control clients readable.
	Codec    string   `json:"codec,omitempty"`
	MaxJobs  int      `json:"maxJobs,omitempty"`
	Schedule []string `json:"schedule"`
}

type PublicSettings struct {
	Domain   string   `json:"domain"`
	TokenSet bool     `json:"tokenSet"`
	Name     string   `json:"name"`
	CPUCodec string   `json:"cpuCodec"`
	CPUJobs  int      `json:"cpuJobs"`
	GPUCodec string   `json:"gpuCodec"`
	GPUJobs  int      `json:"gpuJobs"`
	Schedule []string `json:"schedule"`
}

func (s Settings) Public() PublicSettings {
	s = s.Normalized()
	return PublicSettings{Domain: s.Domain, TokenSet: s.Token != "", Name: s.Name, CPUCodec: s.CPUCodec, CPUJobs: s.CPUJobs, GPUCodec: s.GPUCodec, GPUJobs: s.GPUJobs, Schedule: s.Schedule}
}

func (s Settings) Normalized() Settings {
	if s.CPUCodec == "" && s.GPUCodec == "" && (s.Codec != "" || s.MaxJobs != 0) {
		if s.Codec == workerconfig.AV1NVENC {
			s.GPUCodec, s.GPUJobs = s.Codec, s.MaxJobs
		} else {
			s.CPUCodec, s.CPUJobs = s.Codec, s.MaxJobs
		}
	}
	if s.CPUCodec == "" {
		s.CPUCodec = workerconfig.AV1SVT
	}
	if s.GPUCodec == "" {
		s.GPUCodec = workerconfig.AV1NVENC
	}
	return s
}

type Profile struct {
	Key   string
	Name  string
	Codec string
	Jobs  int
}

func (s Settings) Profiles() []Profile {
	s = s.Normalized()
	base := s.Name
	if len(base) > 60 {
		base = base[:60]
	}
	return []Profile{{Key: "cpu", Name: base + "-cpu", Codec: s.CPUCodec, Jobs: s.CPUJobs}, {Key: "gpu", Name: base + "-gpu", Codec: s.GPUCodec, Jobs: s.GPUJobs}}
}

func (s Settings) Validate() error {
	s = s.Normalized()
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
	if s.CPUJobs < 0 || s.GPUJobs < 0 || s.CPUJobs > 64 || s.GPUJobs > 64 || s.CPUJobs+s.GPUJobs > 64 {
		return errors.New("CPU and GPU concurrent encodes must each be between 0 and 64 and total no more than 64")
	}
	if len(s.Schedule) > 32 {
		return errors.New("too many schedule windows")
	}
	if s.GPUCodec != workerconfig.AV1NVENC {
		return errors.New("GPU profile must use AV1 NVENC")
	}
	if s.CPUCodec == workerconfig.AV1NVENC {
		return errors.New("CPU profile cannot use AV1 NVENC")
	}
	for _, profile := range s.Profiles() {
		c := &workerconfig.Config{MaxActiveJobs: profile.Jobs, EncodeConfig: &workerconfig.FFMPEGConfig{VideoCodec: profile.Codec}, Schedule: &workerconfig.ScheduleConfig{TimeZone: "Local", Windows: s.Schedule}}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("%s profile: %w", profile.Key, err)
		}
	}
	return nil
}

type Persisted struct {
	Active       *Settings `json:"active,omitempty"`
	Pending      *Settings `json:"pending,omitempty"`
	PendingLimit bool      `json:"pendingLimit,omitempty"`
	Paused       bool      `json:"paused,omitempty"`
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
		legacy := state.Active.CPUCodec == "" && state.Active.GPUCodec == "" && (state.Active.Codec != "" || state.Active.MaxJobs != 0)
		state.Active = ptrSettings(state.Active.Normalized())
		if legacy {
			state.PendingLimit = true
		}
		if err = state.Active.Validate(); err != nil {
			return state, fmt.Errorf("invalid saved settings: %w", err)
		}
	}
	if state.Pending != nil {
		state.Pending = ptrSettings(state.Pending.Normalized())
		if err = state.Pending.Validate(); err != nil {
			return state, fmt.Errorf("invalid pending settings: %w", err)
		}
	}
	return state, nil
}

func ptrSettings(s Settings) *Settings { return &s }

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
	return writeWorkerConfig(filepath.Join(dir, "config.yaml"), value)
}

func WorkerConfigForProfile(dir string, s Settings, profile Profile, paused bool) error {
	maxJobs := profile.Jobs
	if paused {
		maxJobs = 0
	}
	value := map[string]any{"web": map[string]any{"domain": s.Domain, "token": s.Token}, "worker": map[string]any{"name": profile.Name, "maxActiveJobs": maxJobs, "temporalPath": filepath.Join(dir, "work"), "ffmpegConfig": map[string]any{"videoCodec": profile.Codec, "parallelJobs": max(1, profile.Jobs)}, "schedule": map[string]any{"timeZone": "Local", "windows": s.Schedule}}}
	return writeWorkerConfig(filepath.Join(dir, "config-"+profile.Key+".yaml"), value)
}

func writeWorkerConfig(path string, value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	return writeAtomic(dir, filepath.Base(path), data)
}
