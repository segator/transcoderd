package step

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteEmptySRT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.srt")

	err := writeEmptySRT(path)
	if err != nil {
		t.Fatalf("writeEmptySRT() error = %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if fi.Size() == 0 {
		t.Errorf("writeEmptySRT() file size = 0, want non-empty valid SRT content")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	want := "1\n00:00:00,000 --> 00:00:00,001\n \n\n"
	if string(content) != want {
		t.Errorf("writeEmptySRT() content = %q, want %q", string(content), want)
	}
}

func TestMinPGSFileSize(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int
		wantSkip bool
	}{
		{"empty file", 0, true},
		{"140 bytes header only", 140, true},
		{"below threshold", 1023, true},
		{"at threshold", 1024, false},
		{"above threshold", 2048, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := int64(tt.fileSize) < minPGSFileSize
			if got != tt.wantSkip {
				t.Errorf("size %d < minPGSFileSize = %v, want %v", tt.fileSize, got, tt.wantSkip)
			}
		})
	}
}
