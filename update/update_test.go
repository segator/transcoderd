package update

import (
	"testing"
	"time"

	"github.com/blang/semver/v4"
)

func TestCleanVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Version with v prefix", "v1.2.3", "1.2.3"},
		{"Version without v", "1.2.3", "1.2.3"},
		{"Empty string", "", ""},
		{"Only v", "v", ""},
		{"Version with V uppercase", "V1.2.3", "V1.2.3"}, // Should only remove lowercase v
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanVersion(tt.input)
			if got != tt.want {
				t.Errorf("cleanVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewUpdater(t *testing.T) {
	interval := time.Minute * 15
	tests := []struct {
		name        string
		version     string
		assetName   string
		noUpdates   bool
		tmpPath     string
		wantErr     bool
		errContains string
	}{
		{
			name:      "Valid version",
			version:   "v1.2.3",
			assetName: "transcoderd-server",
			noUpdates: false,
			tmpPath:   "/tmp",
			wantErr:   false,
		},
		{
			name:      "Valid version without v",
			version:   "1.2.3",
			assetName: "transcoderd-worker",
			noUpdates: true,
			tmpPath:   "/tmp",
			wantErr:   false,
		},
		{
			name:        "Invalid version",
			version:     "invalid",
			assetName:   "transcoderd-server",
			noUpdates:   false,
			tmpPath:     "/tmp",
			wantErr:     true,
			errContains: "Invalid Semantic Version",
		},
		{
			name:      "Empty version",
			version:   "",
			assetName: "transcoderd-server",
			noUpdates: false,
			tmpPath:   "/tmp",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewUpdater(tt.version, tt.assetName, tt.noUpdates, tt.tmpPath, &interval)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewUpdater() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewUpdater() unexpected error = %v", err)
				return
			}

			if got == nil {
				t.Fatal("NewUpdater() returned nil")
			}

			if got.assetName != tt.assetName {
				t.Errorf("assetName = %s, want %s", got.assetName, tt.assetName)
			}

			if got.tmpPath != tt.tmpPath {
				t.Errorf("tmpPath = %s, want %s", got.tmpPath, tt.tmpPath)
			}

			if got.noUpdates != tt.noUpdates {
				t.Errorf("noUpdates = %v, want %v", got.noUpdates, tt.noUpdates)
			}
		})
	}
}

func TestUpdaterFields(t *testing.T) {
	interval := time.Minute * 10
	updater, err := NewUpdater("v2.0.0", "test-asset", true, "/tmp/test", &interval)
	if err != nil {
		t.Fatalf("NewUpdater() error = %v", err)
	}

	expectedVersion := semver.MustParse("2.0.0")
	if !updater.currentVersion.EQ(expectedVersion) {
		t.Errorf("currentVersion = %v, want %v", updater.currentVersion, expectedVersion)
	}

	if updater.checkInterval == nil {
		t.Fatal("checkInterval should not be nil")
	}

	if *updater.checkInterval != time.Minute*10 {
		t.Errorf("checkInterval = %v, want %v", *updater.checkInterval, time.Minute*10)
	}
}

func TestGitHubReleaseStructure(t *testing.T) {
	release := GitHubRelease{
		TagName: "v1.0.0",
		Assets: []GitHubAsset{
			{Name: "asset1", BrowserDownloadURL: "http://example.com/asset1"},
			{Name: "asset2", BrowserDownloadURL: "http://example.com/asset2"},
		},
	}

	if release.TagName != "v1.0.0" {
		t.Errorf("TagName = %s, want v1.0.0", release.TagName)
	}

	if len(release.Assets) != 2 {
		t.Errorf("len(Assets) = %d, want 2", len(release.Assets))
	}

	if release.Assets[0].Name != "asset1" {
		t.Errorf("Assets[0].Name = %s, want asset1", release.Assets[0].Name)
	}
}

func TestGitHubAssetStructure(t *testing.T) {
	asset := GitHubAsset{
		Name:               "transcoderd-server",
		BrowserDownloadURL: "https://github.com/example/download",
	}

	if asset.Name != "transcoderd-server" {
		t.Errorf("Name = %s, want transcoderd-server", asset.Name)
	}

	if asset.BrowserDownloadURL != "https://github.com/example/download" {
		t.Errorf("BrowserDownloadURL = %s, want https://github.com/example/download", asset.BrowserDownloadURL)
	}
}

// Test that version comparison works correctly
func TestVersionComparison(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		isNewer bool
	}{
		{"Patch update", "1.0.0", "1.0.1", true},
		{"Minor update", "1.0.0", "1.1.0", true},
		{"Major update", "1.0.0", "2.0.0", true},
		{"Same version", "1.0.0", "1.0.0", false},
		{"Older version", "2.0.0", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := semver.MustParse(tt.current)
			latest := semver.MustParse(tt.latest)

			got := latest.GT(current)
			if got != tt.isNewer {
				t.Errorf("Version %s > %s = %v, want %v", tt.latest, tt.current, got, tt.isNewer)
			}
		})
	}
}
