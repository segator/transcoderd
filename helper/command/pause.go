package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

var ErrEncodeNotRunning = errors.New("job has no active encoding process")

// PauseController owns only commands explicitly attached to it. Its zero value
// is ready for use. Starts and process removal are serialized with transitions.
type PauseController struct {
	mu        sync.Mutex
	paused    bool
	changed   chan struct{}
	processes map[*os.Process]*pausableProcess
}

type pausableProcess struct {
	key        string
	paused     bool
	individual bool
	active     func() (bool, error)
	setPaused  func(bool) error
	close      func()
}

type EncodePauseState struct {
	General    bool
	Paused     bool
	Individual bool
}

func (p *PauseController) transitionLocked(target func(*pausableProcess) bool) error {
	type transition struct {
		process *pausableProcess
		before  bool
	}
	var changed []transition
	for _, process := range p.processes {
		desired := target(process)
		if process.paused == desired {
			continue
		}
		if err := process.setPaused(desired); err != nil {
			errs := []error{fmt.Errorf("change encoding process pause: %w", err)}
			for i := len(changed) - 1; i >= 0; i-- {
				previous := changed[i]
				if err := previous.process.setPaused(previous.before); err != nil {
					errs = append(errs, fmt.Errorf("rollback encoding pause: %w", err))
				} else {
					previous.process.paused = previous.before
				}
			}
			return errors.Join(errs...)
		}
		changed = append(changed, transition{process, process.paused})
		process.paused = desired
	}
	return nil
}

func (p *PauseController) notifyLocked() {
	if p.changed != nil {
		close(p.changed)
	}
	p.changed = make(chan struct{})
}

func (p *PauseController) SetPaused(paused bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.transitionLocked(func(process *pausableProcess) bool { return paused || process.individual }); err != nil {
		return err
	}
	p.paused = paused
	p.notifyLocked()
	return nil
}

// SetJobPaused changes only the explicit hold of an owned, active encoding.
// General pause still takes precedence; no intent is saved for future processes.
func (p *PauseController) SetJobPaused(key string, paused bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	found := false
	for _, process := range p.processes {
		if process.key != key || key == "" {
			continue
		}
		if process.active != nil {
			active, err := process.active()
			if err != nil {
				return fmt.Errorf("inspect encoding process: %w", err)
			}
			if !active {
				return ErrEncodeNotRunning
			}
		}
		found = true
	}
	if !found {
		return ErrEncodeNotRunning
	}
	if err := p.transitionLocked(func(process *pausableProcess) bool {
		if process.key == key {
			return p.paused || paused
		}
		return process.paused
	}); err != nil {
		return err
	}
	for _, process := range p.processes {
		if process.key == key {
			process.individual = paused
		}
	}
	return nil
}

// ResumeAll releases both kinds of live hold so maintenance can drain.
func (p *PauseController) ResumeAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.transitionLocked(func(*pausableProcess) bool { return false }); err != nil {
		return err
	}
	p.paused = false
	for _, process := range p.processes {
		process.individual = false
	}
	p.notifyLocked()
	return nil
}

func (p *PauseController) Suspended() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, process := range p.processes {
		if process.paused {
			n++
		}
	}
	return n
}

func (p *PauseController) PauseStates() map[string]EncodePauseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	jobs := make(map[string]EncodePauseState)
	for _, process := range p.processes {
		if process.key != "" {
			state := jobs[process.key]
			state.General = p.paused
			state.Paused = state.Paused || process.paused
			state.Individual = state.Individual || process.individual
			jobs[process.key] = state
		}
	}
	return jobs
}

func (p *PauseController) start(ctx context.Context, cmd *exec.Cmd, attach func() error, key string) (func(), error) {
	for {
		p.mu.Lock()
		if !p.paused {
			break
		}
		if p.changed == nil {
			p.changed = make(chan struct{})
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	process, err := openPausableProcess(cmd.Process)
	if err == nil {
		err = attach()
	}
	if err != nil {
		if process != nil {
			process.close()
		}
		_ = cmd.Cancel()
		_ = cmd.Wait()
		return nil, err
	}
	if p.processes == nil {
		p.processes = make(map[*os.Process]*pausableProcess)
	}
	process.key = key
	p.processes[cmd.Process] = process
	return func() { p.mu.Lock(); defer p.mu.Unlock(); delete(p.processes, cmd.Process); process.close() }, nil
}

func (p *PauseController) SuspendedJobs() map[string]bool {
	jobs := make(map[string]bool)
	for key, state := range p.PauseStates() {
		if state.Paused {
			jobs[key] = true
		}
	}
	return jobs
}
