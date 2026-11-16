package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workerconfig "transcoder/worker/config"

	"github.com/spf13/viper"
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

func TestProtectedProfileConfigurationAndRedaction(t *testing.T) {
	settings := Settings{Domain: "https://server.test", Token: "fleet-secret", Name: "node", CPUJobs: 1, GPUJobs: 1,
		CPUCredential: "cpu-secret", CPUWorkspaceID: "7c395c98-0407-4d0b-b636-71de9c05c975",
		GPUCredential: "gpu-secret", GPUWorkspaceID: "91d3b22a-95e9-41fa-ac26-e9e84c051214"}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	public, err := json.Marshal(settings.Public())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{settings.Token, settings.CPUCredential, settings.GPUCredential} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("public settings disclose a credential: %s", public)
		}
	}
	dir := t.TempDir()
	for _, profile := range settings.Profiles() {
		if err := WorkerConfigForProfile(dir, settings, profile, false); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "config-"+profile.Key+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var actual struct {
			Worker struct {
				Protected *workerconfig.ProtectedConfig `json:"protected"`
			} `json:"worker"`
		}
		if err := json.Unmarshal(data, &actual); err != nil {
			t.Fatal(err)
		}
		credential, identity := settings.CPUCredential, settings.CPUWorkspaceID
		if profile.Key == "gpu" {
			credential, identity = settings.GPUCredential, settings.GPUWorkspaceID
		}
		if actual.Worker.Protected == nil || actual.Worker.Protected.Credential != credential || actual.Worker.Protected.WorkspaceID != identity {
			t.Fatalf("%s profile has incorrect protected configuration", profile.Key)
		}
		parser := viper.New()
		parser.SetConfigType("json")
		if err := parser.ReadConfig(strings.NewReader(string(data))); err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Worker *workerconfig.Config `mapstructure:"worker"`
		}
		if err := parser.Unmarshal(&decoded); err != nil || decoded.Worker == nil || decoded.Worker.Protected == nil || decoded.Worker.Protected.Credential != credential || decoded.Worker.Protected.WorkspaceID != identity {
			t.Fatalf("%s profile is not decoded by the worker: %v", profile.Key, err)
		}
	}
	for _, mutate := range []func(*Settings){
		func(s *Settings) { s.CPUWorkspaceID = "" },
		func(s *Settings) { s.GPUCredential = "" },
		func(s *Settings) { s.GPUCredential = s.CPUCredential },
		func(s *Settings) { s.GPUWorkspaceID = strings.ToUpper(s.CPUWorkspaceID) },
	} {
		invalid := settings
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatal("invalid or shared protected profile configuration was accepted")
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

func TestPauseModePersistsAndReachesBothProfiles(t *testing.T) {
	dir := t.TempDir()
	settings := Settings{Domain: "https://server.test", Token: "secret", Name: "node", CPUJobs: 1, GPUJobs: 2, PauseMode: "ram"}.Normalized()
	if err := Save(dir, Persisted{Active: &settings, Paused: true, RAMPaused: true}); err != nil {
		t.Fatal(err)
	}
	saved, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Active.PauseMode != "ram" || saved.Active.Public().PauseMode != "ram" || !saved.Paused {
		t.Fatalf("saved pause = %+v", saved)
	}
	for _, profile := range saved.Active.Profiles() {
		if err := WorkerConfigForProfile(dir, *saved.Active, profile, true, saved.RAMPaused); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "config-"+profile.Key+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var value struct {
			Worker struct {
				PauseMode     string
				RAMPaused     bool
				MaxActiveJobs int
			}
		}
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		if value.Worker.PauseMode != "finish" || value.Worker.MaxActiveJobs != 0 || !value.Worker.RAMPaused {
			t.Fatalf("%s config = %s", profile.Key, data)
		}
	}
	if got := (Settings{}).Normalized().PauseMode; got != "ram" {
		t.Fatalf("legacy desktop pause strategy = %s, want ram", got)
	}
	settings.PauseMode = "chunked"
	if err := settings.Validate(); err == nil {
		t.Fatal("Validate() accepted unimplemented pause mode")
	}
}

func TestIndependentSavedPausePreferences(t *testing.T) {
	for _, queue := range []bool{false, true} {
		for _, ram := range []bool{false, true} {
			dir := t.TempDir()
			settings := Settings{Domain: "https://server.test", Token: "secret", Name: "matrix", CPUJobs: 1, GPUJobs: 2}
			if err := Save(dir, Persisted{Active: &settings, Paused: queue, RAMPaused: ram}); err != nil {
				t.Fatal(err)
			}
			saved, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Paused != queue || saved.RAMPaused != ram || saved.Active.PauseMode != "ram" {
				t.Fatalf("queue=%t RAM=%t saved=%+v", queue, ram, saved)
			}
			for _, profile := range saved.Active.Profiles() {
				if err := WorkerConfigForProfile(dir, *saved.Active, profile, saved.Paused, saved.RAMPaused); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, "config-"+profile.Key+".yaml"))
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					Worker struct {
						MaxActiveJobs int
						RAMPaused     bool
						PauseMode     string
					}
				}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatal(err)
				}
				parser := viper.New()
				parser.SetConfigType("json")
				if err := parser.ReadConfig(strings.NewReader(string(data))); err != nil {
					t.Fatal(err)
				}
				var actual struct {
					Worker *workerconfig.Config `mapstructure:"worker"`
				}
				if err := parser.Unmarshal(&actual); err != nil {
					t.Fatal(err)
				}
				if actual.Worker == nil || actual.Worker.RAMPaused != ram {
					t.Fatalf("worker startup lost RAM preference: %s", data)
				}
				want := profile.Jobs
				if queue {
					want = 0
				}
				if result.Worker.MaxActiveJobs != want || result.Worker.RAMPaused != ram || result.Worker.PauseMode != "finish" {
					t.Fatalf("independent startup flags=%s", data)
				}
			}
		}
	}
}
