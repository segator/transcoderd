package step

import (
	"strings"
	"testing"
	"time"

	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

func TestFFMPEGVerifyRejectsLargerOrTruncatedOutput(t *testing.T) {
	tests := []struct {
		name       string
		sourceSize int64
		targetSize int64
		targetTime time.Duration
		wantError  string
	}{
		{"smaller and complete", 1000, 800, 2 * time.Minute, ""},
		{"same size and complete", 1000, 1000, 2 * time.Minute, ""},
		{"larger output", 1000, 1001, 2 * time.Minute, "size"},
		{"truncated output", 1000, 800, time.Minute, "duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context := &job.Context{
				Source: &job.VideoData{Size: tt.sourceSize, FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{Duration: 2 * time.Minute}}},
				Target: &job.VideoData{Size: tt.targetSize, FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{Duration: tt.targetTime}}},
			}
			err := (&FFMPEGVerifyStep{verifyDeltaTimeSeconds: 1}).verifyJob(context)
			if tt.wantError == "" && err != nil {
				t.Errorf("verifyJob() error = %v, want nil", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Errorf("verifyJob() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}
