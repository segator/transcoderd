// Package assignment implements credentials for protected job assignments.
package assignment

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strings"
)

var (
	ErrInvalidCredentials = errors.New("invalid worker credentials")
	ErrUnknownKey         = errors.New("assignment signing key is unavailable")
)

// Credentials holds hashes of independently provisioned worker tokens. A fleet
// token or a name supplied by the caller is not a worker identity.
type Credentials struct {
	workers map[string][sha256.Size]byte
}

func NewCredentials(hashes map[string]string) (*Credentials, error) {
	c := &Credentials{workers: make(map[string][sha256.Size]byte, len(hashes))}
	seen := make(map[[sha256.Size]byte]string, len(hashes))
	for name, encoded := range hashes {
		if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
			return nil, errors.New("worker identity must not be empty or contain surrounding whitespace")
		}
		value, err := hex.DecodeString(encoded)
		if err != nil || len(value) != sha256.Size {
			return nil, fmt.Errorf("invalid credential hash for worker %q", name)
		}
		var digest [sha256.Size]byte
		copy(digest[:], value)
		if other, ok := seen[digest]; ok {
			return nil, fmt.Errorf("workers %q and %q share a credential", other, name)
		}
		seen[digest], c.workers[name] = name, digest
	}
	return c, nil
}

func (c *Credentials) Authenticate(worker, token string) error {
	provided := sha256.Sum256([]byte(token))
	expected, exists := c.workers[worker]
	matched := subtle.ConstantTimeCompare(expected[:], provided[:]) == 1
	if !exists || token == "" || !matched {
		return ErrInvalidCredentials
	}
	return nil
}

// Keyring keeps retired keys available while protected assignments reference
// them. It copies keys so caller mutations cannot silently change capabilities.
type Keyring struct {
	active string
	keys   map[string][]byte
}

func NewKeyring(active string, keys map[string][]byte) (*Keyring, error) {
	k := &Keyring{active: active, keys: make(map[string][]byte, len(keys))}
	for id, key := range keys {
		if id == "" || strings.TrimSpace(id) != id || len(key) < 32 {
			return nil, fmt.Errorf("invalid assignment signing key %q", id)
		}
		k.keys[id] = append([]byte(nil), key...)
	}
	if _, ok := k.keys[active]; !ok {
		return nil, ErrUnknownKey
	}
	return k, nil
}

func (k *Keyring) ActiveKeyID() string { return k.active }

// Capability returns the same opaque value for an assignment after a lost
// response or server restart. The generation must increase when ownership is
// revoked or a replacement worker claims the assignment.
func (k *Keyring) Capability(keyID, worker, jobID string, generation uint64) (string, error) {
	key, ok := k.keys[keyID]
	if !ok {
		return "", ErrUnknownKey
	}
	if worker == "" || jobID == "" || generation == 0 {
		return "", errors.New("assignment identity is incomplete")
	}
	mac := hmac.New(sha256.New, key)
	writePart(mac, "transcoderd-assignment-v1")
	writePart(mac, keyID)
	writePart(mac, worker)
	writePart(mac, jobID)
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], generation)
	_, _ = mac.Write(n[:])
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func writePart(h hash.Hash, part string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(part)))
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(part))
}

func (k *Keyring) Verify(keyID, worker, jobID string, generation uint64, capability string) error {
	want, err := k.Capability(keyID, worker, jobID, generation)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(want), []byte(capability)) {
		return ErrInvalidCredentials
	}
	return nil
}
