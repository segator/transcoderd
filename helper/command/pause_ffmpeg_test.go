package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt-in native encode check: use the project's FFmpeg on a machine with NVENC.
// Encoded elementary streams are compared, avoiding random Matroska metadata.
func TestRAMPauseFFmpegOutput(t *testing.T) {
	ffmpeg := os.Getenv("TRANSCODERD_PAUSE_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("set TRANSCODERD_PAUSE_TEST_FFMPEG to run CPU/GPU encoding comparisons")
	}
	tests := []struct {
		codec, format, pixel string
		args                 []string
	}{
		{"libsvtav1", "ivf", "yuv420p10le", []string{"-preset", "4", "-crf", "25", "-svtav1-params", "lp=2"}},
		{"av1_nvenc", "ivf", "p010le", []string{"-preset", "p7", "-tune", "uhq", "-rc", "vbr", "-cq", "38", "-b:v", "0", "-multipass", "fullres", "-bf", "4", "-b_ref_mode", "middle", "-split_encode_mode", "disabled", "-qmin", "0", "-qmax", "100"}},
		{"libx265", "hevc", "yuv420p10le", []string{"-preset", "medium", "-crf", "21", "-profile:v", "main10", "-x265-params", "pools=2:frame-threads=1"}},
	}
	for _, tt := range tests {
		t.Run(tt.codec, func(t *testing.T) {
			dir := t.TempDir()
			encode := func(mode string) []byte {
				path := filepath.Join(dir, mode+"."+tt.format)
				args := []string{"-hide_banner", "-loglevel", "error", "-filter_threads", "1", "-re", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8", "-an", "-threads", "2", "-pix_fmt", tt.pixel, "-c:v", tt.codec}
				args = append(args, tt.args...)
				args = append(args, "-stats_period", "0.1", "-progress", "pipe:1", "-nostdin", "-f", tt.format, "-y", path)
				controller := &PauseController{}
				progress := make(chan struct{}, 1)
				var stderr strings.Builder
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				cmd := NewCommand(ffmpeg, args...).SetStderrFunc(func(b []byte, _ bool) { stderr.Write(b) })
				if mode != "continuous" {
					cmd.SetPauseController(controller, "encode")
				}
				pending := ""
				cmd.SetStdoutFunc(func(b []byte, _ bool) {
					pending += string(b)
					for {
						line, rest, ok := strings.Cut(pending, "\n")
						if !ok {
							break
						}
						pending = rest
						if strings.HasPrefix(line, "frame=") {
							n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "frame=")))
							if n > 0 {
								select {
								case progress <- struct{}{}:
								default:
								}
							}
						}
					}
				})
				done := make(chan error, 1)
				go func() { _, err := cmd.RunWithContext(ctx); done <- err }()
				if mode != "continuous" {
					select {
					case <-progress:
					case err := <-done:
						t.Fatalf("encode exited before pause: %v %s", err, stderr.String())
					case <-ctx.Done():
						t.Fatal("encoder produced no progress")
					}
					setPaused := controller.SetPaused
					if mode == "individual" {
						setPaused = func(paused bool) error { return controller.SetJobPaused("encode", paused) }
					}
					if err := setPaused(true); err != nil {
						t.Fatal(err)
					}
					if got := controller.Suspended(); got != 1 {
						t.Fatalf("suspended encoders=%d,want 1", got)
					}
					time.Sleep(300 * time.Millisecond)
					before := pauseFileSize(path)
					time.Sleep(200 * time.Millisecond)
					if got := pauseFileSize(path); got != before {
						t.Fatalf("output grew during pause: %d -> %d", before, got)
					}
					if err := setPaused(false); err != nil {
						t.Fatal(err)
					}
				}
				if err := <-done; err != nil {
					t.Fatalf("FFmpeg: %v %s", err, stderr.String())
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			continuous := encode("continuous")
			for _, scope := range []string{"general", "individual"} {
				paused := encode(scope)
				if len(continuous) == 0 || !bytes.Equal(continuous, paused) {
					t.Fatalf("%s compressed stream mismatch: continuous %d bytes, paused %d bytes", scope, len(continuous), len(paused))
				}
				t.Logf("%s pause identical compressed stream: %d bytes, SHA-256 %x", scope, len(paused), sha256.Sum256(paused))
			}

		})
	}
}
