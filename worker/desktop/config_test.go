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
