package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestResolveVideoSettings(t *testing.T) {
	tests := []struct {
		name string
		cfg  FFMPEGConfig
		want VideoSettings
	}{
		{
			name: "default GPU",
			cfg:  FFMPEGConfig{},
			want: VideoSettings{Codec: AV1NVENC, Preset: "p7", PixelFormat: "p010le", CQ: 34, QMax: 100},
		},
		{
			name: "GPU quality override",
			cfg:  FFMPEGConfig{VideoCodec: AV1NVENC, VideoPreset: "p6", VideoCQ: 30, VideoQMax: 96},
			want: VideoSettings{Codec: AV1NVENC, Preset: "p6", PixelFormat: "p010le", CQ: 30, QMax: 96},
		},
		{
			name: "CPU default",
			cfg:  FFMPEGConfig{VideoCodec: AV1SVT},
			want: VideoSettings{Codec: AV1SVT, Preset: "3", PixelFormat: "yuv420p10le", CRF: 23},
		},
		{
			name: "CPU quality override",
			cfg:  FFMPEGConfig{VideoCodec: AV1SVT, VideoPreset: "4", VideoCRF: 27},
			want: VideoSettings{Codec: AV1SVT, Preset: "4", PixelFormat: "yuv420p10le", CRF: 27},
		},
		{
			name: "x265 rollback",
			cfg:  FFMPEGConfig{VideoCodec: HEVCX265},
			want: VideoSettings{Codec: HEVCX265, Preset: "medium", Profile: "main10", PixelFormat: "yuv420p10le", CRF: 21},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.ResolveVideoSettings()
			if err != nil {
				t.Fatalf("ResolveVideoSettings() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveVideoSettings() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolveVideoSettingsRejectsIncompatibleOptions(t *testing.T) {
	tests := []struct {
		name string
		cfg  FFMPEGConfig
	}{
		{"unknown codec", FFMPEGConfig{VideoCodec: "libx264"}},
		{"GPU with x265 CRF", FFMPEGConfig{VideoCodec: AV1NVENC, VideoCRF: 21}},
		{"GPU with x265 profile", FFMPEGConfig{VideoCodec: AV1NVENC, VideoProfile: "main10"}},
		{"GPU bad preset", FFMPEGConfig{VideoCodec: AV1NVENC, VideoPreset: "medium"}},
		{"GPU bad CQ", FFMPEGConfig{VideoCodec: AV1NVENC, VideoCQ: 64}},
		{"CPU with GPU quality", FFMPEGConfig{VideoCodec: AV1SVT, VideoCQ: 34}},
		{"CPU bad preset", FFMPEGConfig{VideoCodec: AV1SVT, VideoPreset: "p7"}},
		{"CPU bad CRF", FFMPEGConfig{VideoCodec: AV1SVT, VideoCRF: 64}},
		{"x265 with GPU QP", FFMPEGConfig{VideoCodec: HEVCX265, VideoQMax: 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.cfg.ResolveVideoSettings(); err == nil {
				t.Errorf("ResolveVideoSettings() error = nil, want configuration error")
			}
		})
	}
}

func TestWorkerConfigValidation(t *testing.T) {
	valid := Config{MaxActiveJobs: 2, EncodeConfig: &FFMPEGConfig{ParallelJobs: 2}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	tests := []struct {
		name string
		cfg  Config
	}{
		{"zero active jobs", Config{EncodeConfig: &FFMPEGConfig{ParallelJobs: 2}}},
		{"zero parallel encodes", Config{MaxActiveJobs: 2, EncodeConfig: &FFMPEGConfig{}}},
		{"unsupported codec", Config{MaxActiveJobs: 2, EncodeConfig: &FFMPEGConfig{ParallelJobs: 2, VideoCodec: "unknown"}}},
		{"excess threads", Config{MaxActiveJobs: 2, EncodeConfig: &FFMPEGConfig{ParallelJobs: 2, Threads: 256}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Errorf("Validate() error = nil, want configuration error")
			}
		})
	}
}

func TestWorkerAV1YAMLOverrides(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	configFile := `worker:
  maxActiveJobs: 4
  ffmpegConfig:
    videoCodec: libsvtav1
    videoCRF: 27
    parallelJobs: 2
`
	if err := v.ReadConfig(strings.NewReader(configFile)); err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}
	var parsed struct {
		Worker Config `mapstructure:"worker"`
	}
	if err := v.Unmarshal(&parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	settings, err := parsed.Worker.EncodeConfig.ResolveVideoSettings()
	if err != nil {
		t.Fatalf("ResolveVideoSettings() error = %v", err)
	}
	if parsed.Worker.MaxActiveJobs != 4 || parsed.Worker.EncodeConfig.ParallelJobs != 2 || settings.Codec != AV1SVT || settings.CRF != 27 {
		t.Errorf("YAML worker = %+v, video = %+v", parsed.Worker, settings)
	}
}
