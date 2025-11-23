package config

import (
	"testing"
	"time"
)

func TestPGSConfigDefaults(t *testing.T) {
	cfg := &PGSConfig{}

	if cfg.ParallelJobs != 0 {
		t.Errorf("ParallelJobs default = %d, want 0", cfg.ParallelJobs)
	}

	if cfg.DLLPath != "" {
		t.Errorf("DLLPath default = %s, want empty", cfg.DLLPath)
	}
}

func TestFFMPEGConfigDefaults(t *testing.T) {
	cfg := &FFMPEGConfig{}

	if cfg.AudioCodec != "" {
		t.Errorf("AudioCodec default = %s, want empty", cfg.AudioCodec)
	}

	if cfg.VideoCodec != "" {
		t.Errorf("VideoCodec default = %s, want empty", cfg.VideoCodec)
	}

	if cfg.Threads != 0 {
		t.Errorf("Threads default = %d, want 0", cfg.Threads)
	}
}

func TestFFMPEGConfigFields(t *testing.T) {
	cfg := &FFMPEGConfig{
		AudioCodec:   "aac",
		AudioVBR:     5,
		VideoCodec:   "libx264",
		VideoPreset:  "medium",
		VideoProfile: "high",
		VideoCRF:     23,
		Threads:      4,
		ExtraArgs:    "-tune film",
	}

	if cfg.AudioCodec != "aac" {
		t.Errorf("AudioCodec = %s, want aac", cfg.AudioCodec)
	}

	if cfg.AudioVBR != 5 {
		t.Errorf("AudioVBR = %d, want 5", cfg.AudioVBR)
	}

	if cfg.VideoCodec != "libx264" {
		t.Errorf("VideoCodec = %s, want libx264", cfg.VideoCodec)
	}

	if cfg.VideoCRF != 23 {
		t.Errorf("VideoCRF = %d, want 23", cfg.VideoCRF)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{}

	if cfg.TemporalPath != "" {
		t.Errorf("TemporalPath default = %s, want empty", cfg.TemporalPath)
	}

	if cfg.Name != "" {
		t.Errorf("Name default = %s, want empty", cfg.Name)
	}

	if cfg.Paused {
		t.Error("Paused should be false by default")
	}

	if cfg.VerifyDeltaTime != 0 {
		t.Errorf("VerifyDeltaTime default = %f, want 0", cfg.VerifyDeltaTime)
	}
}

func TestConfigHaveSettedPeriodTime(t *testing.T) {
	tests := []struct {
		name       string
		startAfter time.Duration
		stopAfter  time.Duration
		want       bool
	}{
		{
			name:       "Both zero",
			startAfter: 0,
			stopAfter:  0,
			want:       false,
		},
		{
			name:       "StartAfter set",
			startAfter: time.Hour,
			stopAfter:  0,
			want:       true,
		},
		{
			name:       "StopAfter set",
			startAfter: 0,
			stopAfter:  time.Hour,
			want:       true,
		},
		{
			name:       "Both set",
			startAfter: time.Hour,
			stopAfter:  time.Hour * 2,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				StartAfter: &tt.startAfter,
				StopAfter:  &tt.stopAfter,
			}

			got := cfg.HaveSettedPeriodTime()
			if got != tt.want {
				t.Errorf("HaveSettedPeriodTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigWithAllFields(t *testing.T) {
	startAfter := time.Hour
	stopAfter := time.Hour * 2

	cfg := &Config{
		TemporalPath:    "/tmp/worker",
		Name:            "worker-1",
		StartAfter:      &startAfter,
		StopAfter:       &stopAfter,
		Paused:          true,
		VerifyDeltaTime: 0.5,
		PGSConfig: &PGSConfig{
			ParallelJobs: 2,
			DLLPath:      "/path/to/dll",
		},
		EncodeConfig: &FFMPEGConfig{
			AudioCodec:  "aac",
			VideoCodec:  "libx264",
			VideoPreset: "fast",
			VideoCRF:    20,
			Threads:     8,
		},
	}

	if cfg.TemporalPath != "/tmp/worker" {
		t.Errorf("TemporalPath = %s, want /tmp/worker", cfg.TemporalPath)
	}

	if cfg.Name != "worker-1" {
		t.Errorf("Name = %s, want worker-1", cfg.Name)
	}

	if !cfg.Paused {
		t.Error("Paused should be true")
	}

	if cfg.VerifyDeltaTime != 0.5 {
		t.Errorf("VerifyDeltaTime = %f, want 0.5", cfg.VerifyDeltaTime)
	}

	if cfg.PGSConfig == nil {
		t.Fatal("PGSConfig should not be nil")
	}

	if cfg.PGSConfig.ParallelJobs != 2 {
		t.Errorf("PGSConfig.ParallelJobs = %d, want 2", cfg.PGSConfig.ParallelJobs)
	}

	if cfg.EncodeConfig == nil {
		t.Fatal("EncodeConfig should not be nil")
	}

	if cfg.EncodeConfig.VideoCodec != "libx264" {
		t.Errorf("EncodeConfig.VideoCodec = %s, want libx264", cfg.EncodeConfig.VideoCodec)
	}
}

func TestPGSConfigAllFields(t *testing.T) {
	cfg := &PGSConfig{
		ParallelJobs:      4,
		DLLPath:           "/usr/lib/pgs.dll",
		TesseractDataPath: "/usr/share/tessdata",
		DotnetPath:        "/usr/bin/dotnet",
		TessVersion:       5,
		LibleptName:       "liblept",
		LibleptVersion:    180,
	}

	if cfg.ParallelJobs != 4 {
		t.Errorf("ParallelJobs = %d, want 4", cfg.ParallelJobs)
	}

	if cfg.DLLPath != "/usr/lib/pgs.dll" {
		t.Errorf("DLLPath = %s, want /usr/lib/pgs.dll", cfg.DLLPath)
	}

	if cfg.TessVersion != 5 {
		t.Errorf("TessVersion = %d, want 5", cfg.TessVersion)
	}

	if cfg.LibleptVersion != 180 {
		t.Errorf("LibleptVersion = %d, want 180", cfg.LibleptVersion)
	}
}
