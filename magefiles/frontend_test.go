package main

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendIndexPlatforms(t *testing.T) {
	tests := []struct {
		name     string
		configs  []string
		declared []string
		nested   bool
		wantErr  bool
	}{
		{name: "both architectures", configs: []string{"amd64", "arm64"}, declared: []string{"amd64", "arm64"}},
		{name: "nested image index", configs: []string{"amd64", "arm64"}, declared: []string{"amd64", "arm64"}, nested: true},
		{name: "missing ARM64", configs: []string{"amd64"}, declared: []string{"amd64"}, wantErr: true},
		{name: "mislabeled ARM64", configs: []string{"amd64", "amd64"}, declared: []string{"amd64", "arm64"}, wantErr: true},
		{name: "unexpected architecture", configs: []string{"amd64", "s390x"}, declared: []string{"amd64", "s390x"}, wantErr: true},
		{name: "empty index", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFrontendIndex(t, tt.configs, tt.declared, tt.nested)
			if err := checkFrontendIndex(path); (err != nil) != tt.wantErr {
				t.Errorf("checkFrontendIndex() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func writeFrontendIndex(t *testing.T, configs, declared []string, nested bool) string {
	t.Helper()
	files := map[string][]byte{}
	add := func(document any) string {
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		files["blobs/sha256/"+digest] = data
		return "sha256:" + digest
	}
	var manifests []map[string]any
	for i, architecture := range configs {
		config := add(map[string]any{"os": "linux", "architecture": architecture})
		manifest := add(map[string]any{"schemaVersion": 2, "config": map[string]string{"digest": config}})
		manifests = append(manifests, map[string]any{"digest": manifest, "platform": map[string]string{"os": "linux", "architecture": declared[i]}})
	}
	if nested {
		digest := add(map[string]any{"schemaVersion": 2, "manifests": manifests})
		manifests = []map[string]any{{"digest": digest}}
	}
	index, err := json.Marshal(map[string]any{"schemaVersion": 2, "manifests": manifests})
	if err != nil {
		t.Fatal(err)
	}
	files["index.json"] = index
	path := filepath.Join(t.TempDir(), "frontend.tar")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	for name, data := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
