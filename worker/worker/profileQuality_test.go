package worker

import (
	"os"
	"path/filepath"
	"testing"

	"transcoder/worker/config"
	"transcoder/worker/job"

	"github.com/google/uuid"
)

func TestSVTQualitySurvivesProfilePersistenceAndDefaultChanges(t *testing.T) {
	tests := []struct {
		name  string
		saved *config.FFMPEGConfig
		want  int
	}{
		{name: "new implicit default", want: 26},
		{name: "saved legacy implicit default", saved: &config.FFMPEGConfig{VideoCodec: config.AV1SVT}, want: 25},
		{name: "saved explicit previous default", saved: &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoCRF: 25}, want: 25},
		{name: "saved new default", saved: &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoCRF: 26}, want: 26},
		{name: "saved quality override", saved: &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoCRF: 27}, want: 27},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := filepath.Join(os.TempDir(), "transcoderd-profile-"+uuid.NewString())
			defer os.RemoveAll(work)
			j := job.NewContext(uuid.New(), 0, work)
			if err := j.Init(); err != nil {
				t.Fatal(err)
			}
			j.EncodeConfig = tt.saved
			path := filepath.Join(work, j.JobId.String()+".json")
			if err := j.PersistJobContext(); err != nil {
				t.Fatal(err)
			}
			loaded, err := job.ReadContextFromDiskByPath(path)
			if err != nil {
				t.Fatal(err)
			}
			e := &JobExecutor{workerConfig: &config.Config{EncodeConfig: &config.FFMPEGConfig{VideoCodec: config.AV1NVENC}}}
			if err := e.pinEncoderProfile(loaded, config.CPUEncoderKind); err != nil {
				t.Fatal(err)
			}
			if loaded.EncodeConfig.VideoCRF != tt.want {
				t.Fatalf("pinEncoderProfile() CRF = %d, want %d", loaded.EncodeConfig.VideoCRF, tt.want)
			}
			if err := loaded.PersistJobContext(); err != nil {
				t.Fatal(err)
			}
			loaded, err = job.ReadContextFromDiskByPath(path)
			if err != nil {
				t.Fatal(err)
			}
			e.workerConfig.CPUEncodeConfig = &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoCRF: 31}
			if err := e.pinEncoderProfile(loaded, config.CPUEncoderKind); err != nil {
				t.Fatal(err)
			}
			if loaded.EncodeConfig.VideoCRF != tt.want || loaded.EncodeKind != config.CPUEncoderKind {
				t.Errorf("restored profile = %+v / %s, want CRF%d / CPU", loaded.EncodeConfig, loaded.EncodeKind, tt.want)
			}
		})
	}
}
