package config

import (
	"time"
)

type PGSConfig struct {
	ParallelJobs      int    `mapstructure:"parallelJobs" envconfig:"WORKER_PGS_PARALLELJOBS"`
	DLLPath           string `mapstructure:"DLLPath" envconfig:"WORKER_PGS_TO_SRT_DLL_PATH"`
	TesseractDataPath string `mapstructure:"tessdataPath" envconfig:"WORKER_TESSERACT_DATA_PATH"`
	DotnetPath        string `mapstructure:"dotnetPath" envconfig:"WORKER_DOTNET_PATH"`
	TessVersion       int    `mapstructure:"tessVersion" envconfig:"WORKER_TESS_VERSION"`
	LibleptName       string `mapstructure:"libleptName" envconfig:"WORKER_LIBLEPT_NAME"`
	LibleptVersion    int    `mapstructure:"libleptVersion" envconfig:"WORKER_LIBLEPT_VERSION"`
}

type FFMPEGConfig struct {
	AudioCodec   string `mapstructure:"audioCodec" envconfig:"WORKER_FFMPEG_AUDIOCODEC"`
	AudioVBR     int    `mapstructure:"audioVBR" envconfig:"WORKER_FFMPEG_AUDIOVBR"`
	VideoCodec   string `mapstructure:"videoCodec" envconfig:"WORKER_FFMPEG_VIDEOCODEC"`
	VideoPreset  string `mapstructure:"videoPreset" envconfig:"WORKER_FFMPEG_VIDEOPRESET"`
	VideoProfile string `mapstructure:"videoProfile" envconfig:"WORKER_FFMPEG_VIDEOPROFILE"`
	VideoCRF     int    `mapstructure:"videoCRF" envconfig:"WORKER_FFMPEG_VIDEOCRF"`
}

type Config struct {
	TemporalPath string         `mapstructure:"temporalPath" envconfig:"WORKER_TMP_PATH"`
	Name         string         `mapstructure:"name" envconfig:"WORKER_NAME"`
	Threads      int            `mapstructure:"threads" envconfig:"WORKER_THREADS"`
	Priority     int            `mapstructure:"priority" envconfig:"WORKER_PRIORITY"`
	StartAfter   *time.Duration `mapstructure:"startAfter" envconfig:"WORKER_START_AFTER"`
	StopAfter    *time.Duration `mapstructure:"stopAfter" envconfig:"WORKER_STOP_AFTER"`
	Paused       bool
	PGSConfig    *PGSConfig    `mapstructure:"pgsConfig"`
	EncodeConfig *FFMPEGConfig `mapstructure:"ffmpegConfig"`
}

func (c Config) HaveSettedPeriodTime() bool {
	return *c.StartAfter != 0 || *c.StopAfter != 0
}
