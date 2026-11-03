package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"transcoder/update/managed"
)

func run(args []string) error {
	if len(args) == 1 && args[0] == "key-check" {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("WINDOWS_UPDATE_SIGNING_KEY")))
		if err != nil || len(b) != ed25519.PrivateKeySize {
			return errors.New("release signing secret is missing or invalid")
		}
		key := ed25519.PrivateKey(b)
		pub, err := managed.PublicKey()
		if err != nil {
			return err
		}
		probe := []byte("TranscoderD Windows release signing probe v1")
		if !ed25519.Verify(pub, probe, ed25519.Sign(key, probe)) {
			return errors.New("release signing secret does not match the pinned public key")
		}
		return nil
	}
	if len(args) == 3 && args[0] == "files" {
		files, err := managed.InstallationFiles(args[1])
		if err != nil {
			return err
		}
		return managed.SaveJSON(args[2], files)
	}
	if len(args) == 2 && args[0] == "verify" {
		data, err := os.ReadFile(filepath.Join(args[1], managed.ManifestName))
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(filepath.Join(args[1], managed.SignatureName))
		if err != nil {
			return err
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
		if err != nil {
			return err
		}
		key, err := managed.PublicKey()
		if err != nil {
			return err
		}
		manifest, err := managed.Verify(data, decoded, key)
		if err != nil {
			return err
		}
		return managed.VerifyFile(filepath.Join(args[1], manifest.Installer.Name), manifest.Installer)
	}
	f := flag.NewFlagSet("windows-update-manifest", flag.ContinueOnError)
	version := f.String("version", "", "release tag")
	installer := f.String("installer", "", "complete installer")
	filesPath := f.String("files", "", "installed file descriptor")
	out := f.String("output", "", "output directory")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *version == "" || *installer == "" || *filesPath == "" || *out == "" {
		return errors.New("provide --version, --installer, --files and --output")
	}
	keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("WINDOWS_UPDATE_SIGNING_KEY")))
	if err != nil || len(keyBytes) != ed25519.PrivateKeySize {
		return errors.New("WINDOWS_UPDATE_SIGNING_KEY is missing or invalid")
	}
	key := ed25519.PrivateKey(keyBytes)
	pub, err := managed.PublicKey()
	if err != nil {
		return err
	}
	if !bytes.Equal(pub, key.Public().(ed25519.PublicKey)) {
		return errors.New("release signing key does not match the pinned application key")
	}
	var files []managed.File
	b, err := os.ReadFile(*filesPath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &files); err != nil {
		return err
	}
	asset, err := managed.DescribeFile(*installer, filepath.Base(*installer))
	if err != nil {
		return err
	}
	m := managed.Manifest{Schema: 1, Version: *version, Architecture: "amd64", Installer: asset, Files: files}
	if err = m.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	signature := ed25519.Sign(key, data)
	if _, err = managed.Verify(data, signature, pub); err != nil {
		return err
	}
	if err = os.MkdirAll(*out, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(*out, managed.ManifestName), data, 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, managed.SignatureName), []byte(base64.StdEncoding.EncodeToString(signature)+"\n"), 0600)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
