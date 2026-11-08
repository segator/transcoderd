package step

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asticode/go-astisub"
)

var webVTTTimingLine = regexp.MustCompile(`^[ \t]*(?:[0-9]+:)?[0-9]{2}:[0-9]{2}\.[0-9]{3}[ \t]+-->[ \t]+(?:[0-9]+:)?[0-9]{2}:[0-9]{2}\.[0-9]{3}(?:[ \t].*)?$`)
var srtTimingLine = regexp.MustCompile(`^[ \t]*[0-9]+:[0-9]{2}:[0-9]{2}[,.][0-9]{3}[ \t]+-->[ \t]+[0-9]+:[0-9]{2}:[0-9]{2}[,.][0-9]{3}(?:[ \t].*)?$`)

// prepareExtractedSubtitle normalizes whitespace-only WebVTT separators. FFmpeg
// can otherwise exit successfully with no cues for mkvextract's padded WebVTT.
// The original extraction is retained until the normal job workspace cleanup.
func prepareExtractedSubtitle(path string) (input string, expectedCues int, cleanup func(), err error) {
	cleanup = func() {}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, cleanup, fmt.Errorf("read extracted subtitle: %w", err)
	}
	header := bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	firstLine, _, _ := strings.Cut(string(header), "\n")
	if fields := strings.Fields(firstLine); len(fields) == 0 || fields[0] != "WEBVTT" {
		return path, -1, cleanup, nil
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(header), "\r\n", "\n"), "\r", "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
		}
	}
	var normalized []string
	hasUntimedText := false
	for _, block := range strings.Split(strings.Join(lines, "\n"), "\n\n") {
		block = strings.Trim(block, "\n")
		if block == "" {
			continue
		}
		blockLines := strings.Split(block, "\n")
		first := blockLines[0]
		if (len(normalized) == 0 && strings.HasPrefix(first, "WEBVTT")) || first == "NOTE" || strings.HasPrefix(first, "NOTE ") || first == "STYLE" || first == "REGION" {
			normalized = append(normalized, block)
			continue
		}
		timingIndex := -1
		for i, line := range blockLines {
			if webVTTTimingLine.MatchString(line) {
				timingIndex = i
				break
			}
		}
		if timingIndex == 0 || timingIndex == 1 {
			expectedCues++
			normalized = append(normalized, block)
			continue
		}
		if strings.Contains(block, "-->") {
			return "", 0, cleanup, fmt.Errorf("invalid converted subtitle: malformed WebVTT timing block")
		}
		// Codec-private WebVTT can contain untimed annotation blocks. They
		// are not playable cues; omitting them preserves every timed cue and
		// lets FFmpeg continue instead of stopping at the first orphan block.
		hasUntimedText = true
	}
	if expectedCues == 0 && hasUntimedText {
		return "", 0, cleanup, fmt.Errorf("invalid converted subtitle: nonempty WebVTT has no timed cues")
	}
	f, err := os.CreateTemp(filepath.Dir(path), "subtitle-*.vtt")
	if err != nil {
		return "", 0, cleanup, fmt.Errorf("create normalized WebVTT: %w", err)
	}
	cleanup = func() { _ = os.Remove(f.Name()) }
	if _, err := f.WriteString(strings.Join(normalized, "\n\n") + "\n"); err != nil {
		_ = f.Close()
		cleanup()
		return "", 0, cleanup, fmt.Errorf("write normalized WebVTT: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", 0, cleanup, fmt.Errorf("close normalized WebVTT: %w", err)
	}
	return f.Name(), expectedCues, cleanup, nil
}

func validateConvertedSRT(path string, expectedCues int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read converted subtitle: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		if expectedCues > 0 {
			return fmt.Errorf("invalid converted subtitle: expected %d cues, got empty output", expectedCues)
		}
		return writeEmptySRT(path)
	}
	subtitles, err := parseConvertedSRT(data)
	if err != nil {
		return fmt.Errorf("invalid converted subtitle: %w", err)
	}
	if expectedCues >= 0 && len(subtitles.Items) != expectedCues {
		return fmt.Errorf("invalid converted subtitle: expected %d cues, got %d", expectedCues, len(subtitles.Items))
	}
	return nil
}

func parseConvertedSRT(data []byte) (*astisub.Subtitles, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"), "\n")
	timingLines := 0
	for i, line := range lines {
		if srtTimingLine.MatchString(line) {
			timingLines++
			continue
		}
		if !strings.Contains(line, "-->") {
			continue
		}
		// The upstream parser treats any arrow as a timing header, including
		// arrows recognized by OCR inside dialogue. Escape only cue text: a
		// malformed header after a fresh numeric index remains an error.
		newCue := i > 0 && isSRTIndex(lines[i-1]) && (i == 1 || strings.TrimSpace(lines[i-2]) == "")
		if timingLines == 0 || newCue {
			return nil, fmt.Errorf("invalid SRT timing at line %d", i+1)
		}
		// Span tags are ignored by astisub and keep the arrow out of its
		// header detector while preserving text, entities and current styling.
		lines[i] = strings.ReplaceAll(line, "-->", "--<span>></span>")
	}
	subtitles, err := astisub.ReadFromSRT(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		return nil, err
	}
	if len(subtitles.Items) == 0 {
		return nil, fmt.Errorf("no valid subtitle cues")
	}
	if len(subtitles.Items) != timingLines {
		return nil, fmt.Errorf("parsed %d of %d subtitle cues", len(subtitles.Items), timingLines)
	}
	return subtitles, nil
}

func isSRTIndex(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	for _, c := range line {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func writeConvertedSRT(path string, subtitles *astisub.Subtitles) error {
	f, err := os.CreateTemp(filepath.Dir(path), "subtitle-*.srt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := subtitles.WriteToSRT(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
