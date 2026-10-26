package helper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundlePathWithSpacesAndUnicode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Transcoder películas")
	path := filepath.Join(root, "bin", "ffmpeg.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tool"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := bundlePath(root, "bin/ffmpeg.exe", "ffmpeg"); got != path {
		t.Fatalf("bundlePath() = %q, want %q", got, path)
	}
	if got := bundlePath(root, "missing.exe", "fallback"); got != "fallback" {
		t.Fatalf("bundlePath() = %q, want fallback", got)
	}
}

func TestValidExtension(t *testing.T) {
	tests := []struct {
		name      string
		extension string
		want      bool
	}{
		{
			name:      "Valid extension mp4",
			extension: "mp4",
			want:      true,
		},
		{
			name:      "Valid extension mkv",
			extension: "mkv",
			want:      true,
		},
		{
			name:      "Valid extension avi",
			extension: "avi",
			want:      true,
		},
		{
			name:      "Invalid extension txt",
			extension: "txt",
			want:      false,
		},
		{
			name:      "Invalid extension jpg",
			extension: "jpg",
			want:      false,
		},
		{
			name:      "Empty extension",
			extension: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidExtension(tt.extension)
			if got != tt.want {
				t.Errorf("ValidExtension(%q) = %v, want %v", tt.extension, got, tt.want)
			}
		})
	}
}

func TestGetFFmpegPath(t *testing.T) {
	path := GetFFmpegPath()
	if path != "ffmpeg" {
		t.Errorf("GetFFmpegPath() = %q, want %q", path, "ffmpeg")
	}
}

func TestGetMKVExtractPath(t *testing.T) {
	path := GetMKVExtractPath()
	if path != "mkvextract" {
		t.Errorf("GetMKVExtractPath() = %q, want %q", path, "mkvextract")
	}
}
