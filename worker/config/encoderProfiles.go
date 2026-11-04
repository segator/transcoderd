package config

import "fmt"

const (
	CPUEncoderKind = "cpu"
	GPUEncoderKind = "gpu"
)

func EncoderKind(codec string) string {
	if codec == AV1NVENC || codec == "" {
		return GPUEncoderKind
	}
	return CPUEncoderKind
}

// EncoderProfiles copies the primary recipe, and uses approved defaults for the
// other class. Quality flags and hardware-specific extraArgs never cross classes.
func (c *Config) EncoderProfiles() (cpu, gpu *FFMPEGConfig, err error) {
	if c == nil || c.EncodeConfig == nil {
		return nil, nil, fmt.Errorf("worker.ffmpegConfig is required")
	}
	primary := *c.EncodeConfig
	common := FFMPEGConfig{AudioCodec: primary.AudioCodec, AudioVBR: primary.AudioVBR, Threads: primary.Threads}
	cpuConfig, gpuConfig := common, common
	cpuConfig.VideoCodec, gpuConfig.VideoCodec = AV1SVT, AV1NVENC
	if EncoderKind(primary.VideoCodec) == GPUEncoderKind {
		gpuConfig = primary
	} else {
		cpuConfig = primary
	}
	if c.CPUEncodeConfig != nil {
		cpuConfig = *c.CPUEncodeConfig
		if cpuConfig.VideoCodec == "" {
			cpuConfig.VideoCodec = AV1SVT
		}
	}
	if c.GPUEncodeConfig != nil {
		gpuConfig = *c.GPUEncodeConfig
		if gpuConfig.VideoCodec == "" {
			gpuConfig.VideoCodec = AV1NVENC
		}
	}
	for _, profile := range []*FFMPEGConfig{&cpuConfig, &gpuConfig} {
		if profile.AudioCodec == "" {
			profile.AudioCodec = primary.AudioCodec
		}
		if profile.AudioVBR == 0 {
			profile.AudioVBR = primary.AudioVBR
		}
	}
	for _, profile := range []struct {
		kind   string
		config *FFMPEGConfig
	}{{CPUEncoderKind, &cpuConfig}, {GPUEncoderKind, &gpuConfig}} {
		resolved, err := profile.config.ResolveVideoSettings()
		if err != nil {
			return nil, nil, fmt.Errorf("%s encoder profile: %w", profile.kind, err)
		}
		if EncoderKind(resolved.Codec) != profile.kind {
			return nil, nil, fmt.Errorf("%s encoder profile cannot use %s", profile.kind, resolved.Codec)
		}
		if profile.config.Threads < 0 || profile.config.Threads > 255 {
			return nil, nil, fmt.Errorf("%s profile threads must be 0 through 255", profile.kind)
		}
	}
	return &cpuConfig, &gpuConfig, nil
}
