package scheduler

import (
	"testing"
)

func TestVerifyFailureMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "mov_text subtitle now retries",
			message: "error: Subtitle: mov_text is not supported",
			want:    true,
		},
		{
			name:    "WebVTT codec now retries",
			message: "unsupported AVCodecID S_TEXT/WEBVTT in stream 3",
			want:    true,
		},
		{
			name:    "disk quota exceeded retries",
			message: "Disk quota exceeded",
			want:    true,
		},
		{
			name:    "no space left retries",
			message: "No space left on device",
			want:    true,
		},
		{
			name:    "job not found does not retry",
			message: "job not found",
			want:    false,
		},
		{
			name:    "index out of range does not retry",
			message: "runtime error: index out of range [3] with length 2",
			want:    false,
		},
		{
			name:    "source file size does not retry",
			message: "source File size mismatch",
			want:    false,
		},
		{
			name:    "404 download does not retry",
			message: "not 200 respose in dowload code 404",
			want:    false,
		},
		{
			name:    "mkvextract error retries",
			message: "MKVExtract unexpected error during extraction",
			want:    true,
		},
		{
			name:    "download 500 retries",
			message: "download code 500",
			want:    true,
		},
		{
			name:    "connection refused retries",
			message: "connection refused",
			want:    true,
		},
		{
			name:    "checksum error retries",
			message: "CHecksum error on download source",
			want:    true,
		},
		{
			name:    "GLIBC not found retries",
			message: "GLIBC_2.34 not found",
			want:    true,
		},
		{
			name:    "received signal 15 retries",
			message: "received signal 15",
			want:    true,
		},
		{
			name:    "bin_data does not retry",
			message: "Data: bin_data found in stream",
			want:    false,
		},
		{
			name:    "scale/rate 0/0 does not retry",
			message: "scale/rate is 0/0 which is invalid",
			want:    false,
		},
		{
			name:    "probably corrupt does not retry",
			message: "probably corrupt input file",
			want:    false,
		},
		{
			name:    "with 0 items does not retry",
			message: "completed with 0 items",
			want:    false,
		},
		{
			name:    "exit status 1 empty stderr retries",
			message: "exit status 1: stder: stdout:",
			want:    true,
		},
		{
			name:    "nil pointer retries",
			message: "runtime error: invalid memory address or nil pointer dereference",
			want:    true,
		},
		{
			name:    "maybe incorrect parameters retries",
			message: "maybe incorrect parameters such as bit_rate, rate, width or height",
			want:    true,
		},
		{
			name:    "no such host retries",
			message: "dial tcp: lookup foo.bar: no such host",
			want:    true,
		},
		{
			name:    "i/o timeout retries",
			message: "i/o timeout connecting to server",
			want:    true,
		},
		{
			name:    "Errorf while decoding does not retry",
			message: "Errorf while decoding stream #0:1",
			want:    false,
		},
		{
			name:    "source file duration does not retry",
			message: "source File duration mismatch",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyFailureMessage(tt.message)
			if got != tt.want {
				t.Errorf("verifyFailureMessage(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestFormatTargetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "x264 to x265 mkv",
			input: "movies/test.x264.avi",
			want:  "movies/test.x265.mkv",
		},
		{
			name:  "h264 to x265 mkv",
			input: "movies/test.h264.mp4",
			want:  "movies/test.x265.mkv",
		},
		{
			name:  "ac3 to AAC",
			input: "movies/test.ac3.mkv",
			want:  "movies/test.AAC.mkv",
		},
		{
			name:  "no codec match same extension returns unchanged",
			input: "movies/test.mkv",
			want:  "movies/test.mkv",
		},
		{
			name:  "no codec match different extension changes to mkv",
			input: "movies/test.avi",
			want:  "movies/test.mkv",
		},
		{
			name:  "multiple replacements",
			input: "shows/Season 1/show.x264.dts.avi",
			want:  "shows/Season 1/show.x265.AAC.mkv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTargetName(tt.input)
			if got != tt.want {
				t.Errorf("formatTargetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
