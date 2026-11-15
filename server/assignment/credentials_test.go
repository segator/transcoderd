package assignment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestCredentialsAreBoundToWorker(t *testing.T) {
	first := sha256.Sum256([]byte("worker-one-token"))
	second := sha256.Sum256([]byte("worker-two-token"))
	credentials, err := NewCredentials(map[string]string{"one": hex.EncodeToString(first[:]), "two": hex.EncodeToString(second[:])})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, token string
		allowed     bool
	}{{"one", "worker-one-token", true}, {"two", "worker-one-token", false}, {"missing", "worker-one-token", false}, {"one", "", false}} {
		err := credentials.Authenticate(tt.name, tt.token)
		if (err == nil) != tt.allowed {
			t.Errorf("Authenticate(%q) = %v, allowed = %v", tt.name, err, tt.allowed)
		}
	}
	if _, err := NewCredentials(map[string]string{"one": hex.EncodeToString(first[:]), "two": hex.EncodeToString(first[:])}); err == nil {
		t.Fatal("duplicate credentials must not identify different workers")
	}
}

func TestCapabilityFencesAndSurvivesKeyRotation(t *testing.T) {
	oldKey := bytes.Repeat([]byte{1}, 32)
	keys := map[string][]byte{"old": oldKey, "new": bytes.Repeat([]byte{2}, 32)}
	ring, err := NewKeyring("new", keys)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := ring.Capability("old", "worker", "job", 4)
	if err != nil {
		t.Fatal(err)
	}
	oldKey[0] = 3 // Input keys cannot mutate a running keyring.
	if err := ring.Verify("old", "worker", "job", 4, capability); err != nil {
		t.Fatalf("Verify() = %v, want success", err)
	}
	for _, tt := range []struct {
		key, worker, job string
		generation       uint64
	}{{"old", "other", "job", 4}, {"old", "worker", "other", 4}, {"old", "worker", "job", 5}, {"new", "worker", "job", 4}} {
		if err := ring.Verify(tt.key, tt.worker, tt.job, tt.generation, capability); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Verify(%+v) = %v, want ErrInvalidCredentials", tt, err)
		}
	}
	if _, err := ring.Capability("missing", "worker", "job", 4); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("Capability() = %v, want ErrUnknownKey", err)
	}
}
