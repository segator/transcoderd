package step

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"transcoder/helper"
	"transcoder/worker/job"
)

const paddedWebVTT = "\xef\xbb\xbfWEBVTT\n\nNOTE SegmentIndex\nNOTE Segment=1.000 50@100 1\n\n    \t  \n\n1\n00:00:01.000 --> 00:00:02.000 position:50%,middle\n<c.bg_transparent>Caption sample</c.bg_transparent>\n<c.bg_transparent>Same timed cue</c.bg_transparent>\n\nUntimed annotation\n\n2\n00:00:03.000 --> 00:00:04.000\nSecond caption\n"

func TestPrepareExtractedSubtitlePreservesCues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "4.extract")
	if err := os.WriteFile(path, []byte(paddedWebVTT), 0600); err != nil {
		t.Fatal(err)
	}
	input, cues, cleanup, err := prepareExtractedSubtitle(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if cues != 2 {
		t.Fatalf("prepareExtractedSubtitle() cues = %d, want 2", cues)
	}
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "    \t  ") || !strings.Contains(string(data), "Caption sample") || !strings.Contains(string(data), "00:00:03.000 --> 00:00:04.000") {
		t.Fatalf("normalized WebVTT lost cues or retained padding: %q", data)
	}
	original, err := os.ReadFile(path)
	if err != nil || string(original) != paddedWebVTT {
		t.Fatalf("original extraction changed: %v", err)
	}
}

func TestPrepareExtractedSubtitleRejectsUnparseableWebVTT(t *testing.T) {
	tests := []string{
		"WEBVTT\n\nUnparseable nonempty track\n",
		"WEBVTT\n\n1\n00:bad --> 00:00:02.000\nCaption\n",
	}
	for _, input := range tests {
		path := filepath.Join(t.TempDir(), "track.extract")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, cleanup, err := prepareExtractedSubtitle(path)
		cleanup()
		if err == nil {
			t.Fatalf("prepareExtractedSubtitle(%q) succeeded, want invalid-input error", input)
		}
	}
}

func TestOCRSubtitleArrowsRemainDialogue(t *testing.T) {
	text := "Take charge. 80 00:14:49,740 --> 00:14:52,300. Caption"
	data := "79\r\n00:15:14,831 --> 00:15:18,126\r\n" + text + "\r\n\r\n80\r\n00:15:32,140 --> 00:15:34,726\r\nNext cue\r\n"
	subtitles, err := parseConvertedSRT([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(subtitles.Items) != 2 || subtitles.Items[0].Lines[0].String() != text || subtitles.Items[0].StartAt != 914831*time.Millisecond {
		t.Fatalf("parseConvertedSRT() = %+v, want original text, timing and both cues", subtitles.Items)
	}
	path := filepath.Join(t.TempDir(), "ocr.srt")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeConvertedSRT(path, subtitles); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := parseConvertedSRT(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Items) != 2 || restored.Items[0].Lines[0].String() != text {
		t.Fatalf("rewritten SRT lost dialogue: %s", raw)
	}
}

func TestInvalidLaterSRTCueIsRejected(t *testing.T) {
	data := "1\n00:00:01,000 --> 00:00:02,000\nValid\n\n2\n00:bad --> 00:00:03,000\nInvalid\n"
	if _, err := parseConvertedSRT([]byte(data)); err == nil {
		t.Fatal("parseConvertedSRT() accepted malformed second cue")
	}
}

func TestValidateConvertedSRT(t *testing.T) {
	valid := "1\n00:00:01,000 --> 00:00:02,000\nCaption\n\n"
	tests := []struct {
		name, input string
		expected    int
		wantError   bool
	}{
		{"valid output", valid, 1, false},
		{"empty genuine track", "", 0, false},
		{"silent dropped cues", "", 85, true},
		{"partial conversion", valid, 2, true},
		{"invalid nonempty output", "not subtitles", -1, true},
		{"invalid timing", "1\n00:bad --> 00:00:02,000\nCaption", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "4.srt")
			if err := os.WriteFile(path, []byte(tt.input), 0600); err != nil {
				t.Fatal(err)
			}
			if err := validateConvertedSRT(path, tt.expected); (err != nil) != tt.wantError {
				t.Fatalf("validateConvertedSRT() = %v, want error %v", err, tt.wantError)
			}
			if !tt.wantError {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := parseConvertedSRT(data); err != nil {
					t.Fatalf("validated SRT is invalid: %v", err)
				}
			}
		})
	}
}

func TestConvertExtractedPaddedWebVTTWithFFmpeg(t *testing.T) {
	if binary := os.Getenv("AV1_TEST_FFMPEG"); binary != "" {
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if _, err := exec.LookPath(helper.GetFFmpegPath()); err != nil {
		t.Skipf("ffmpeg is unavailable: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "4.extract"), []byte(paddedWebVTT), 0600); err != nil {
		t.Fatal(err)
	}
	m := &MKVExtractStepExecutor{}
	if err := m.convertExtractedToSrt(t.Context(), &mockTracker{}, &job.Context{WorkingDir: dir}, 4); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "4.srt"))
	if err != nil {
		t.Fatal(err)
	}
	subtitles, err := parseConvertedSRT(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(subtitles.Items) != 2 || !strings.Contains(string(data), "Caption sample") || !strings.Contains(string(data), "Same timed cue") || !strings.Contains(string(data), "Second caption") || strings.Contains(string(data), "Untimed annotation") {
		t.Fatalf("converted SRT = %q, want both original cues", data)
	}
}
