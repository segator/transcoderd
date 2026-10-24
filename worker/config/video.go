package config

import (
	"fmt"
	"strconv"
)

const (
	AV1NVENC = "av1_nvenc"
	AV1SVT   = "libsvtav1"
	HEVCX265 = "libx265"
)

// VideoSettings holds the resolved, encoder-specific options for one worker.
type VideoSettings struct {
	Codec       string
	Preset      string
	Profile     string
	PixelFormat string
	CRF         int
	CQ          int
	QMax        int
}

func (c *FFMPEGConfig) ResolveVideoSettings() (VideoSettings, error) {
	if c == nil {
		return VideoSettings{}, fmt.Errorf("worker.ffmpegConfig is required")
	}
	codec := c.VideoCodec
	if codec == "" {
		codec = AV1NVENC
	}
	switch codec {
	case AV1NVENC:
		if c.VideoCRF != 0 || c.VideoProfile != "" {
			return VideoSettings{}, fmt.Errorf("av1_nvenc does not accept videoCRF or videoProfile; use videoCQ")
		}
		preset := c.VideoPreset
		if preset == "" {
			preset = "p7"
		}
		if len(preset) != 2 || preset[0] != 'p' || preset[1] < '1' || preset[1] > '7' {
			return VideoSettings{}, fmt.Errorf("av1_nvenc videoPreset must be p1 through p7, got %q", preset)
		}
		cq := c.VideoCQ
		if cq == 0 {
			cq = 38
		}
		if cq < 1 || cq > 63 {
			return VideoSettings{}, fmt.Errorf("av1_nvenc videoCQ must be 1 through 63, got %d", cq)
		}
		qmax := c.VideoQMax
		if qmax == 0 {
			qmax = 100
		}
		if qmax < 1 || qmax > 255 {
			return VideoSettings{}, fmt.Errorf("av1_nvenc videoQMax must be 1 through 255, got %d", qmax)
		}
		return VideoSettings{Codec: codec, Preset: preset, PixelFormat: "p010le", CQ: cq, QMax: qmax}, nil
	case AV1SVT:
		if c.VideoCQ != 0 || c.VideoQMax != 0 || c.VideoProfile != "" {
			return VideoSettings{}, fmt.Errorf("libsvtav1 does not accept videoCQ, videoQMax, or videoProfile; use videoCRF")
		}
		preset := c.VideoPreset
		if preset == "" {
			preset = "4"
		}
		presetNumber, err := strconv.Atoi(preset)
		if err != nil || presetNumber < 0 || presetNumber > 13 {
			return VideoSettings{}, fmt.Errorf("libsvtav1 videoPreset must be 0 through 13, got %q", preset)
		}
		crf := c.VideoCRF
		if crf == 0 {
			crf = 25
		}
		if crf < 1 || crf > 63 {
			return VideoSettings{}, fmt.Errorf("libsvtav1 videoCRF must be 1 through 63, got %d", crf)
		}
		return VideoSettings{Codec: codec, Preset: preset, PixelFormat: "yuv420p10le", CRF: crf}, nil
	case HEVCX265:
		if c.VideoCQ != 0 || c.VideoQMax != 0 {
			return VideoSettings{}, fmt.Errorf("libx265 does not accept videoCQ or videoQMax; use videoCRF")
		}
		preset := c.VideoPreset
		if preset == "" {
			preset = "medium"
		}
		switch preset {
		case "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow", "placebo":
		default:
			return VideoSettings{}, fmt.Errorf("unsupported libx265 videoPreset %q", preset)
		}
		profile := c.VideoProfile
		if profile == "" {
			profile = "main10"
		}
		if profile != "main10" {
			return VideoSettings{}, fmt.Errorf("libx265 videoProfile must be main10 with 10-bit output, got %q", profile)
		}
		crf := c.VideoCRF
		if crf == 0 {
			crf = 21
		}
		if crf < 1 || crf > 51 {
			return VideoSettings{}, fmt.Errorf("libx265 videoCRF must be 1 through 51, got %d", crf)
		}
		return VideoSettings{Codec: codec, Preset: preset, Profile: profile, PixelFormat: "yuv420p10le", CRF: crf}, nil
	default:
		return VideoSettings{}, fmt.Errorf("unsupported worker.ffmpegConfig.videoCodec %q", codec)
	}
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("worker configuration is required")
	}
	if c.MaxActiveJobs < 0 {
		return fmt.Errorf("worker.maxActiveJobs must be non-negative, got %d", c.MaxActiveJobs)
	}
	if c.EncodeConfig == nil {
		return fmt.Errorf("worker.ffmpegConfig is required")
	}
	if c.EncodeConfig.ParallelJobs < 0 {
		return fmt.Errorf("worker.ffmpegConfig.parallelJobs must be nonnegative (0 inherits maxActiveJobs), got %d", c.EncodeConfig.ParallelJobs)
	}
	if c.EncodeConfig.Threads < 0 || c.EncodeConfig.Threads > 255 {
		return fmt.Errorf("worker.ffmpegConfig.threads must be 0 through 255, got %d", c.EncodeConfig.Threads)
	}
	if _, err := c.EncodeConfig.ResolveVideoSettings(); err != nil {
		return err
	}
	return c.validateSchedule()
}
