package repository

import (
	"context"
	"time"
)

type oidcDashboardSession struct {
	credentialID [32]byte
	expires      time.Time
}

func (r *InMemoryRepository) CreateOIDCDashboardSession(_ context.Context, credentialID, hash [32]byte, expires time.Time, limit int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var active int
	for key, session := range r.oidcSessions {
		if !now.Before(session.expires) {
			delete(r.oidcSessions, key)
		} else if session.credentialID == credentialID {
			active++
		}
	}
	if active >= limit {
		return false, nil
	}
	r.oidcSessions[hash] = oidcDashboardSession{credentialID: credentialID, expires: expires}
	return true, nil
}

func (r *InMemoryRepository) HasOIDCDashboardSession(_ context.Context, hash [32]byte) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return time.Now().Before(r.oidcSessions[hash].expires), nil
}

func (r *InMemoryRepository) DeleteOIDCDashboardSession(_ context.Context, hash [32]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.oidcSessions, hash)
	return nil
}
