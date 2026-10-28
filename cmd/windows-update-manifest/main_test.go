package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"transcoder/update/managed"
)

func TestSigningRequiresConfiguredMatchingKey(t *testing.T) {
	t.Setenv("WINDOWS_UPDATE_SIGNING_KEY", "")
	if err := run([]string{"key-check"}); err == nil {
		t.Fatal("key-check accepted a missing signing secret")
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	originalMode, originalKey := managed.FixtureMode, managed.FixtureKey
	managed.FixtureMode, managed.FixtureKey = "true", base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { managed.FixtureMode, managed.FixtureKey = originalMode, originalKey })
	t.Setenv("WINDOWS_UPDATE_SIGNING_KEY", base64.StdEncoding.EncodeToString(key))
	if err = run([]string{"key-check"}); err != nil {
		t.Fatal(err)
	}
	_, wrong, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINDOWS_UPDATE_SIGNING_KEY", base64.StdEncoding.EncodeToString(wrong))
	if err = run([]string{"key-check"}); err == nil {
		t.Fatal("key-check accepted the wrong signing secret")
	}
	out := t.TempDir()
	if err = run([]string{"--version", "v99.1.0", "--installer", "missing.exe", "--files", "missing.json", "--output", out}); err == nil {
		t.Fatal("signing accepted the wrong secret")
	}
	if _, err = os.Stat(filepath.Join(out, managed.ManifestName)); !os.IsNotExist(err) {
		t.Fatal("failed signing published eligible metadata")
	}
}
