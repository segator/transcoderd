package step

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

func TestFFMPEGVerifyRejectsLargerOrTruncatedOutput(t *testing.T) {
	tests := []struct {
		name            string
		sourceSize      int64
		targetSize      int64
		sourceTime      time.Duration
		targetTime      time.Duration
		videoSourceTime time.Duration
		videoTargetTime time.Duration
		probeError      error
		wantError       string
		wantProbeCall   bool
	}{
		{name: "smaller and complete", sourceSize: 1000, targetSize: 800, sourceTime: 2 * time.Minute, targetTime: 2 * time.Minute, videoSourceTime: 2 * time.Minute, videoTargetTime: 2 * time.Minute},
		{name: "same size and complete", sourceSize: 1000, targetSize: 1000, sourceTime: 2 * time.Minute, targetTime: 2 * time.Minute, videoSourceTime: 2 * time.Minute, videoTargetTime: 2 * time.Minute},
		{name: "larger output", sourceSize: 1000, targetSize: 1001, sourceTime: 2 * time.Minute, targetTime: 2 * time.Minute, videoSourceTime: 2 * time.Minute, videoTargetTime: 2 * time.Minute, wantError: "size"},
		{name: "truncated output", sourceSize: 1000, targetSize: 800, sourceTime: 2 * time.Minute, targetTime: time.Minute, videoSourceTime: 2 * time.Minute, videoTargetTime: time.Minute, wantError: "duration", wantProbeCall: true},
		{name: "subtitle tail extends container duration", sourceSize: 1000, targetSize: 800, sourceTime: 6526*time.Second + 528*time.Millisecond, targetTime: 6449*time.Second + 915*time.Millisecond, videoSourceTime: 6449*time.Second + 734*time.Millisecond, videoTargetTime: 6449*time.Second + 915*time.Millisecond, wantProbeCall: true},
		{name: "output non-video stream extends container duration", sourceSize: 1000, targetSize: 800, sourceTime: 5000 * time.Second, targetTime: 5331 * time.Second, videoSourceTime: 5000 * time.Second, videoTargetTime: 5000 * time.Second, wantProbeCall: true},
		{name: "cannot measure mismatched video", sourceSize: 1000, targetSize: 800, sourceTime: 2 * time.Minute, targetTime: time.Minute, probeError: errors.New("invalid packet data"), wantError: "measure source video duration", wantProbeCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobContext := &job.Context{
				Source: &job.VideoData{FilePath: "source.mkv", Size: tt.sourceSize, FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{Duration: tt.sourceTime}}},
				Target: &job.VideoData{FilePath: "target.mkv", Size: tt.targetSize, FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{Duration: tt.targetTime}}},
			}
			probeCalls := 0
			step := &FFMPEGVerifyStep{
				verifyDeltaTimeSeconds: 1,
				probeVideoDuration: func(_ context.Context, path string) (time.Duration, error) {
					probeCalls++
					if tt.probeError != nil {
						return 0, tt.probeError
					}
					if path == "source.mkv" {
						return tt.videoSourceTime, nil
					}
					return tt.videoTargetTime, nil
				},
			}
			err := step.verifyJob(context.Background(), jobContext)
			if tt.wantError == "" && err != nil {
				t.Errorf("verifyJob() error = %v, want nil", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Errorf("verifyJob() error = %v, want %q", err, tt.wantError)
			}
			if got := probeCalls > 0; got != tt.wantProbeCall {
				t.Errorf("probe called = %v, want %v", got, tt.wantProbeCall)
			}
		})
	}
}
