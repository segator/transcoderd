package scheduler

import (
	"sync"
	"time"
)

const sourceReadGuardDuration = 24 * time.Hour

type sourceReadAssignment struct {
	jobID   string
	eventID int
}

// sourceReadGuard limits retries for a single assignment after a source I/O
// failure. It is deliberately independent of client connection failures.
type sourceReadGuard struct {
	mu      sync.Mutex
	blocked map[sourceReadAssignment]time.Time
}

func (g *sourceReadGuard) block(assignment sourceReadAssignment, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.blocked == nil {
		g.blocked = make(map[sourceReadAssignment]time.Time)
	}
	for key, until := range g.blocked {
		if !now.Before(until) {
			delete(g.blocked, key)
		}
	}
	g.blocked[assignment] = now.Add(sourceReadGuardDuration)
}

func (g *sourceReadGuard) isBlocked(assignment sourceReadAssignment, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, found := g.blocked[assignment]
	if found && !now.Before(until) {
		delete(g.blocked, assignment)
		return false
	}
	return found
}
