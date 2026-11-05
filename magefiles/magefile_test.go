package main

import (
	"reflect"
	"testing"
)

func TestParseImageVersionAliases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "trims and removes empty and duplicate aliases",
			input: " v2.5, ,v2,v2.5, ",
			want:  []string{"v2.5", "v2"},
		},
		{
			name:  "empty input",
			input: " , ",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageVersionAliases(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseImageVersionAliases(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestImageTagsFor(t *testing.T) {
	for _, component := range []string{"server", "worker", "frontend"} {
		t.Run(component, func(t *testing.T) {
			got := imageTagsFor(
				"ghcr.io/segator/transcoderd",
				component,
				"v2.5.1",
				"main",
				[]string{" v2.5 ", "", "v2", "v2.5.1", "main"},
			)
			want := []string{
				"ghcr.io/segator/transcoderd:" + component + "-v2.5.1",
				"ghcr.io/segator/transcoderd:" + component + "-main",
				"ghcr.io/segator/transcoderd:" + component + "-v2.5",
				"ghcr.io/segator/transcoderd:" + component + "-v2",
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("imageTagsFor() = %v, want %v", got, want)
			}
		})
	}
}

func TestLinuxReleaseAssetsExcludeUnvalidatedWindowsFiles(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"transcoderd-server", true},
		{"transcoderd-worker", true},
		{"transcoderd-frontend", true},
		{"server-image.tar", true},
		{"worker-image.tar", true},
		{"frontend-image.tar", true},
		{"transcoderd-worker-windows-amd64.exe", false},
		{"transcoderd-worker-v2.12.0-windows-amd64.zip", false},
		{"transcoderd-worker-v2.12.0-windows-amd64-setup.exe", false},
		{"windows-update.json", false},
		{"windows-update.json.sig", false},
		{"windows-update-manifest.exe", false},
		{"key", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLinuxReleaseAsset(tt.name); got != tt.want {
				t.Errorf("isLinuxReleaseAsset(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
