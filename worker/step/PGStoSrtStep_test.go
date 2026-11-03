package step

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"transcoder/worker/config"
)

func TestPGSCommandSelectsNativeExecutableOrAssembly(t *testing.T) {
	tests := []struct {
		name       string
		cfg        config.PGSConfig
		executable string
		args       []string
	}{
		{"assembly", config.PGSConfig{DotnetPath: "dotnet", DLLPath: "PgsToSrt.dll"}, "dotnet", []string{"PgsToSrt.dll", "--input", `C:\películas\film.sup`}},
		{"native", config.PGSConfig{ExecutablePath: `C:\Program Files\OCR\PgsToSrt.exe`, DLLPath: "unused.dll"}, `C:\Program Files\OCR\PgsToSrt.exe`, []string{"--input", `C:\películas\film.sup`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pgsCommand(&tt.cfg, []string{"--input", `C:\películas\film.sup`})
			if got.Command != tt.executable || !reflect.DeepEqual(got.Params, tt.args) {
				t.Fatalf("pgsCommand() = %q %q, want %q %q", got.Command, got.Params, tt.executable, tt.args)
			}
		})
	}
}

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

func TestCalculateTesseractLanguage(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "empty defaults to English", language: "", want: "eng"},
		{name: "Slovenian bibliographic code", language: "slo", want: "slv"},
		{name: "Slovenian terminological code", language: "slv", want: "slv"},
		{name: "Slovenian two-letter code", language: "sl", want: "slv"},
		{name: "Spanish two-letter code", language: "es", want: "spa"},
		{name: "Spanish bibliographic code", language: "esp", want: "spa"},
		{name: "unknown code is preserved", language: "xyz", want: "xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateTesseractLanguage(tt.language); got != tt.want {
				t.Errorf("calculateTesseractLanguage(%q) = %q, want %q", tt.language, got, tt.want)
			}
		})
	}
}
