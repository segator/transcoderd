package config

import "testing"

func TestIndependentProfilesKeepApprovedDefaultsAndOverrides(t *testing.T) {
	c := &Config{EncodeConfig: &FFMPEGConfig{VideoCodec: AV1NVENC, VideoCQ: 40, VideoPreset: "p6", ExtraArgs: "-hwaccel cuda", AudioCodec: "aac", AudioVBR: 5, Threads: 4}}
	cpu, gpu, err := c.EncoderProfiles()
	if err != nil {
		t.Fatal(err)
	}
	cpuVideo, _ := cpu.ResolveVideoSettings()
	gpuVideo, _ := gpu.ResolveVideoSettings()
	if cpuVideo.Codec != AV1SVT || cpuVideo.CRF != 26 || cpuVideo.Preset != "4" || cpu.ExtraArgs != "" || cpu.AudioCodec != "aac" || cpu.Threads != 4 {
		t.Errorf("CPU secondary = %+v / %+v", cpu, cpuVideo)
	}
	if gpuVideo.CQ != 40 || gpuVideo.Preset != "p6" || gpu.ExtraArgs != "-hwaccel cuda" {
		t.Errorf("GPU primary = %+v", gpu)
	}
	c.CPUEncodeConfig = &FFMPEGConfig{VideoCodec: HEVCX265, VideoCRF: 22}
	cpu, _, err = c.EncoderProfiles()
	if err != nil || cpu.VideoCodec != HEVCX265 || cpu.AudioCodec != "aac" {
		t.Errorf("CPU override = %+v, %v", cpu, err)
	}
	for _, kind := range []string{"cpu", "gpu"} {
		t.Run("invalid "+kind, func(t *testing.T) {
			bad := &Config{EncodeConfig: &FFMPEGConfig{VideoCodec: AV1NVENC}}
			if kind == "cpu" {
				bad.CPUEncodeConfig = &FFMPEGConfig{VideoCodec: AV1NVENC}
			} else {
				bad.GPUEncodeConfig = &FFMPEGConfig{VideoCodec: AV1SVT}
			}
			if err := bad.Validate(); err == nil {
				t.Error("Validate() accepted a wrong-class profile")
			}
		})
	}
}
