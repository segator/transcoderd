//go:build integration && windows

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWindowsBundle executes the distributed tools without a global dotnet or
// FFmpeg installation. It is independent of the live-server E2E environment.
func TestWindowsBundle(t *testing.T) {
	bundle, fixture := os.Getenv("WINDOWS_BUNDLE"), os.Getenv("WINDOWS_FIXTURE")
	if bundle == "" || fixture == "" {
		t.Skip("set WINDOWS_BUNDLE and WINDOWS_FIXTURE")
	}
	for _, name := range []string{"PgsToSrt.exe", "coreclr.dll", "hostfxr.dll"} {
		if _, err := os.Stat(filepath.Join(bundle, "pgs", name)); err != nil {
			t.Fatalf("private OCR runtime is missing %s: %v", name, err)
		}
	}
	// A clean PATH catches accidentally missing DLLs and global-runtime fallback.
	t.Setenv("PATH", filepath.Join(os.Getenv("SystemRoot"), "System32"))
	work := filepath.Join(t.TempDir(), "prueba película con espacios")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureData, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixture = filepath.Join(work, "source película.mkv")
	if err := os.WriteFile(fixture, fixtureData, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := func(name string) string { return filepath.Join(bundle, "bin", name+".exe") }
	run := func(t *testing.T, executable string, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Dir = work
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %q: %v\n%s", executable, args, err, out)
		}
		return out
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if out := run(t, tool(name), "-version"); !bytes.Contains(out, []byte("9.0.2")) {
			t.Fatalf("%s version: %s", name, out)
		}
	}
	encoders := string(run(t, tool("ffmpeg"), "-hide_banner", "-encoders"))
	for _, encoder := range []string{"libsvtav1", "libaom-av1", "av1_nvenc", "libx265", "libfdk_aac"} {
		if !strings.Contains(encoders, " "+encoder+" ") {
			t.Errorf("bundle is missing %s", encoder)
		}
	}
	for _, tt := range []struct {
		codec string
		args  []string
	}{
		{"libsvtav1", []string{"-preset", "8", "-svtav1-params", "lp=2"}},
		{"libaom-av1", []string{"-cpu-used", "8", "-threads", "2"}},
		{"libx265", []string{"-preset", "ultrafast", "-profile:v", "main10", "-x265-params", "pools=2:frame-threads=1"}},
	} {
		t.Run(tt.codec, func(t *testing.T) {
			args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x128:rate=2:duration=1", "-pix_fmt", "yuv420p10le", "-c:v", tt.codec}
			args = append(args, tt.args...)
			output := filepath.Join(work, tt.codec+".mkv")
			run(t, tool("ffmpeg"), append(args, "-y", output)...)
			format := strings.TrimSpace(string(run(t, tool("ffprobe"), "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=pix_fmt", "-of", "default=noprint_wrappers=1:nokey=1", output)))
			if format != "yuv420p10le" {
				t.Errorf("%s pixel format = %s, want yuv420p10le", tt.codec, format)
			}
			run(t, tool("ffmpeg"), "-v", "error", "-i", output, "-f", "null", "-")
		})
	}
	run(t, tool("ffmpeg"), "-v", "error", "-f", "lavfi", "-i", "sine=duration=1", "-c:a", "libfdk_aac", "-vbr", "5", "-y", filepath.Join(work, "audio.m4a"))
	var probe struct {
		Streams []struct {
			Index int    `json:"index"`
			Codec string `json:"codec_name"`
			Tags  struct {
				Language string `json:"language"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(run(t, tool("ffprobe"), "-v", "error", "-show_streams", "-of", "json", fixture), &probe); err != nil {
		t.Fatal(err)
	}
	for _, stream := range probe.Streams {
		if stream.Codec != "hdmv_pgs_subtitle" {
			continue
		}
		sup, srt := filepath.Join(work, "subtitle.sup"), filepath.Join(work, "subtitle.srt")
		run(t, tool("mkvextract"), "tracks", fixture, fmt.Sprintf("%d:%s", stream.Index, sup))
		language := stream.Tags.Language
		if language == "" {
			language = "eng"
		}
		out := run(t, filepath.Join(bundle, "pgs", "PgsToSrt.exe"), "--input", sup, "--output", srt,
			"--tesseractlanguage", language, "--tesseractdata", filepath.Join(bundle, "tessdata"))
		data, err := os.ReadFile(srt)
		if err != nil || !bytes.Contains(data, []byte("-->")) {
			t.Fatalf("OCR produced no subtitle cues: %v\n%s\n%s", err, out, data)
		}
		t.Logf("Native OCR passed for %s: %d bytes", language, len(data))
		return
	}
	t.Fatal("fixture has no PGS subtitle to exercise OCR")
}
