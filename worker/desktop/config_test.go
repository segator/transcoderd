package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsValidationAndSecretRedaction(t *testing.T) {
	valid := Settings{Domain: "http://example.test:8080", Token: "secret", Name: "pcisaac", Codec: "av1_nvenc", MaxJobs: 2, Schedule: []string{"22:00-10:00"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(valid.Public())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), valid.Token) {
		t.Fatalf("public settings disclose token: %s", data)
	}
	tests := []struct {
		name   string
		mutate func(*Settings)
	}{
		{"invalid URL", func(s *Settings) { s.Domain = "file:///etc/passwd" }},
		{"URL credentials", func(s *Settings) { s.Domain = "https://user:pass@example.test" }},
		{"bad name", func(s *Settings) { s.Name = "../../escape" }},
		{"bad token", func(s *Settings) { s.Token = "" }},
		{"bad codec", func(s *Settings) { s.Codec = "missing" }},
		{"negative limit", func(s *Settings) { s.MaxJobs = -1 }},
		{"bad schedule", func(s *Settings) { s.Schedule = []string{"25:00-01:00"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := valid
			tt.mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("invalid setting accepted")
			}
		})
	}
}

func TestPersistedPendingSettingsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	active := &Settings{Domain: "https://server.test", Token: "secret", Name: "node", Codec: "libsvtav1", MaxJobs: 2}
	pending := *active
	pending.Name = "next"
	want := Persisted{Active: active, Pending: &pending, PendingLimit: true}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active.Name != "node" || got.Pending.Name != "next" || !got.PendingLimit {
		t.Fatalf("Load() = %+v", got)
	}
	if err := WorkerConfig(dir, *got.Active); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"videoCodec":"libsvtav1"`) {
		t.Fatal("worker config missing codec")
	}
}

func TestMixedProfileSettingsGenerateIndependentWorkerConfigs(t *testing.T) {
	dir := t.TempDir()
	settings := Settings{
		Domain: "https://server.test", Token: "secret", Name: "encode-node",
		CPUCodec: "libsvtav1", CPUJobs: 1, GPUCodec: "av1_nvenc", GPUJobs: 4,
	}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, profile := range settings.Profiles() {
		if err := WorkerConfigForProfile(dir, settings, profile, false); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "config-"+profile.Key+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatal(err)
		}
		worker, ok := config["worker"].(map[string]any)
		if !ok || worker["name"] != profile.Name || worker["maxActiveJobs"] != float64(profile.Jobs) {
			t.Errorf("%s config = %s, missing independent worker settings", profile.Key, data)
		}
		ffmpeg, ok := worker["ffmpegConfig"].(map[string]any)
		if !ok || ffmpeg["videoCodec"] != profile.Codec {
			t.Errorf("%s config = %s, missing profile codec", profile.Key, data)
		}
	}
}

func TestLegacyProfileSettingsMigrateToMatchingEncoder(t *testing.T) {
	for _, tt := range []struct {
		codec   string
		cpuJobs int
		gpuJobs int
	}{
		{codec: "libsvtav1", cpuJobs: 2},
		{codec: "av1_nvenc", gpuJobs: 2},
	} {
		dir := t.TempDir()
		data := `{"active":{"domain":"https://server.test","token":"secret","name":"node","codec":"` + tt.codec + `","maxJobs":2}}`
		if err := os.WriteFile(filepath.Join(dir, "service.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		state, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if state.Active.CPUJobs != tt.cpuJobs || state.Active.GPUJobs != tt.gpuJobs || !state.PendingLimit {
			t.Errorf("Load() migrated profile to CPU=%d GPU=%d pending=%t; want CPU=%d GPU=%d pending=true", state.Active.CPUJobs, state.Active.GPUJobs, state.PendingLimit, tt.cpuJobs, tt.gpuJobs)
		}
	}
}
