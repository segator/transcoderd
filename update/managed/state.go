package managed

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const LockedMessage = "A mandatory update is pending. Configuration is locked until the update completes."

type Status struct {
	Pending          bool      `json:"pending"`
	Phase            string    `json:"phase"`
	TargetVersion    string    `json:"targetVersion,omitempty"`
	InstalledVersion string    `json:"installedVersion"`
	Downloaded       int64     `json:"downloaded,omitempty"`
	Total            int64     `json:"total,omitempty"`
	NextRetry        time.Time `json:"nextRetry,omitempty"`
	LastCheck        time.Time `json:"lastCheck,omitempty"`
	Message          string    `json:"message,omitempty"`
	Attempts         int       `json:"attempts,omitempty"`
}

type Candidate struct {
	Data      []byte `json:"data"`
	Signature []byte `json:"signature"`
	URL       string `json:"url"`
}

type State struct {
	Status
	Candidate *Candidate `json:"candidate,omitempty"`
}

func StatePath(dir string) string { return filepath.Join(dir, "updates", "state.json") }
func Load(dir string) (State, error) {
	var s State
	b, err := os.ReadFile(StatePath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if len(b) > 2*MaxManifestSize {
		return s, errors.New("windows update state too large")
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("read Windows update state: %w", err)
	}
	return s, nil
}

func Save(dir string, s State) error { return SaveJSON(StatePath(dir), s) }

func SaveJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if err := ProtectDir(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".update-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return ReplaceFile(tmp, path)
}

func RetryDelay(attempt int) time.Duration {
	if FixtureMode == "true" {
		return 10 * time.Second
	}
	if attempt <= 1 {
		return 15 * time.Minute
	}
	if attempt == 2 {
		return time.Hour
	}
	return 6 * time.Hour
}

func (s *State) Fail(err error, now time.Time) {
	s.Attempts++
	s.Phase = "retrying"
	s.Message = err.Error()
	s.NextRetry = now.Add(RetryDelay(s.Attempts))
}

func (s State) ConfigurationAllowed(command string) bool {
	return !s.Pending || (command != "configure" && command != "set-max-jobs")
}
