package managed

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Hooks struct {
	Drain   func() (int, int, error)
	Install func(*Candidate, *Manifest, string) error
	Recover func(*Transaction) error
}

type Manager struct {
	mu           sync.RWMutex
	dir, version string
	client       *Client
	hooks        Hooks
	state        State
	check        chan struct{}
	interval     time.Duration
}

func NewManager(dir, version string, client *Client, hooks Hooks) *Manager {
	s, err := Load(dir)
	if err != nil {
		s = State{Status: Status{Pending: true, Phase: "retrying", Message: err.Error()}}
	}
	s.InstalledVersion = version
	interval := time.Hour + time.Duration(rand.IntN(300))*time.Second
	if FixtureMode == "true" {
		interval = 5 * time.Second
	}
	return &Manager{dir: dir, version: version, client: client, hooks: hooks, state: s, check: make(chan struct{}, 1), interval: interval}
}
func (m *Manager) Snapshot() Status { m.mu.RLock(); defer m.mu.RUnlock(); return m.state.Status }
func (m *Manager) CheckNow() {
	select {
	case m.check <- struct{}{}:
	default:
	}
}
func (m *Manager) set(s State) error { m.mu.Lock(); m.state = s; m.mu.Unlock(); return Save(m.dir, s) }
func (m *Manager) failure(s State, err error, now time.Time) error {
	s.Fail(err, now)
	_ = m.set(s)
	return err
}

func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	force := true
	for {
		_ = m.Step(ctx, time.Now(), force)
		force = false
		select {
		case <-ctx.Done():
			return
		case <-m.check:
			force = true
		case <-ticker.C:
		}
	}
}

func (m *Manager) Step(ctx context.Context, now time.Time, force bool) error {
	m.mu.RLock()
	s := m.state
	m.mu.RUnlock()
	if disk, err := Load(m.dir); err == nil {
		if disk.Phase != "" {
			s = disk
			s.InstalledVersion = m.version
			m.mu.Lock()
			m.state = s
			m.mu.Unlock()
		}
	} else {
		s.Pending = true
		return m.failure(s, err, now)
	}
	t, err := LoadTransaction(m.dir)
	if err != nil {
		s.Pending = true
		return m.failure(s, err, now)
	}
	if t.Active() {
		s.Pending = true
		s.Phase = t.Phase
		m.mu.Lock()
		m.state = s
		m.mu.Unlock()
		if m.hooks.Recover != nil {
			return m.hooks.Recover(t)
		}
		return nil
	}
	// Complete the second half of the durable commit after a crash between the
	// transaction commit and clearing the admission barrier.
	if t != nil && t.Phase == "completed" && s.Pending {
		manifest, e := Verify(t.Candidate.Data, t.Candidate.Signature, m.client.Key)
		if e != nil {
			return m.failure(s, e, now)
		}
		installed, e := Version(m.version)
		if e != nil {
			return m.failure(s, e, now)
		}
		target, _ := Version(manifest.Version)
		if installed.EQ(target) {
			return ClearCompleted(m.dir, m.version)
		}
	}
	retryDue := s.NextRetry.IsZero() || !now.Before(s.NextRetry)
	if force || (retryDue && (s.LastCheck.IsZero() || now.Sub(s.LastCheck) >= m.interval || s.Phase == "retrying")) {
		candidate, manifest, err := m.client.Check(ctx, m.version)
		s.LastCheck = now
		if err != nil {
			return m.failure(s, err, now)
		}
		if candidate != nil {
			selectNew := s.Candidate == nil
			if s.Candidate != nil {
				old, e := Verify(s.Candidate.Data, s.Candidate.Signature, m.client.Key)
				if e != nil {
					selectNew = true
				} else {
					v, _ := Version(manifest.Version)
					prev, _ := Version(old.Version)
					selectNew = v.GT(prev)
				}
			}
			if selectNew {
				s.Candidate = candidate
				s.TargetVersion = manifest.Version
				s.Total = manifest.Installer.Size
				s.Downloaded = 0
				s.Attempts = 0
				s.Pending = true
				s.Phase = "pending"
				s.Message = "New jobs are blocked while the mandatory update completes."
				s.NextRetry = time.Time{}
			}
		}
		if !s.Pending {
			s.Phase = "current"
			s.Message = "Your installation is up to date."
			s.NextRetry = time.Time{}
			s.Attempts = 0
		}
		if err = m.set(s); err != nil {
			return m.failure(s, err, now)
		}
	}
	if !s.Pending || !retryDue {
		return nil
	}
	if s.Candidate == nil {
		return m.failure(s, errors.New("update recovery is waiting for authenticated release metadata"), now)
	}
	manifest, err := Verify(s.Candidate.Data, s.Candidate.Signature, m.client.Key)
	if err != nil {
		return m.failure(s, err, now)
	}
	if m.hooks.Drain == nil {
		return m.failure(s, errors.New("worker update controls are unavailable"), now)
	}
	_, _, err = m.hooks.Drain()
	if err != nil {
		return m.failure(s, err, now)
	}
	path := filepath.Join(StageDir(m.dir, manifest), manifest.Installer.Name)
	if VerifyFile(path, manifest.Installer) != nil {
		s.Phase = "downloading"
		s.Message = "Downloading the mandatory update. Accepted jobs are finishing."
		if err = m.set(s); err != nil {
			return m.failure(s, err, now)
		}
		last := time.Time{}
		path, err = m.client.Stage(ctx, m.dir, s.Candidate, func(n int64) {
			s.Downloaded = n
			m.mu.Lock()
			m.state.Downloaded = n
			m.mu.Unlock()
			if time.Since(last) > time.Second {
				_ = m.set(s)
				last = time.Now()
			}
		})
		if err != nil {
			return m.failure(s, err, now)
		}
		s.Downloaded = s.Total
	}
	active, reserved, err := m.hooks.Drain()
	if err != nil {
		return m.failure(s, err, now)
	}
	if active > 0 || reserved > 0 {
		s.Phase = "draining"
		s.Message = "Waiting for all accepted jobs and uploads to finish."
		return m.set(s)
	}
	s.Phase = "installing"
	s.Message = "Installing the mandatory update. Configuration remains locked."
	s.NextRetry = time.Time{}
	if err = m.set(s); err != nil {
		return m.failure(s, err, now)
	}
	if m.hooks.Install == nil {
		return m.failure(s, errors.New("windows installer helper is unavailable"), now)
	}
	if err = m.hooks.Install(s.Candidate, manifest, path); err != nil {
		return m.failure(s, err, now)
	}
	return nil
}

func ClearCompleted(dir, installed string) error {
	s, err := Load(dir)
	if err != nil {
		return err
	}
	s.Pending = false
	s.Phase = "updated"
	s.InstalledVersion = installed
	s.TargetVersion = installed
	s.Message = "The complete installation was updated successfully."
	s.NextRetry = time.Time{}
	s.Attempts = 0
	s.Candidate = nil
	if err = Save(dir, s); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "updates"))
	if err != nil {
		return err
	}
	t, _ := LoadTransaction(dir)
	keep := ""
	if t != nil {
		key, _ := PublicKey()
		if old, e := Verify(t.Candidate.Data, t.Candidate.Signature, key); e == nil {
			keep = old.Installer.SHA256
		}
	}
	for _, entry := range entries {
		if entry.IsDir() && len(entry.Name()) == 64 && entry.Name() != keep {
			path := filepath.Join(dir, "updates", entry.Name())
			if CheckPath(path) == nil {
				_ = os.RemoveAll(path)
			}
		}
	}
	return nil
}
