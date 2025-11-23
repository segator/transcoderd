package ffmpeg

import (
	"strings"
	"testing"
	"time"
)

func TestSubtitleIsImageTypeSubtitle(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{"PGS subtitle uppercase", "PGS", true},
		{"PGS subtitle lowercase", "pgs", true},
		{"PGS subtitle mixed case", "Pgs", true},
		{"PGS in longer string", "hdmv_pgs_subtitle", true},
		{"SRT subtitle", "srt", false},
		{"ASS subtitle", "ass", false},
		{"Empty format", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &Subtitle{Format: tt.format}
			got := sub.IsImageTypeSubtitle()
			if got != tt.want {
				t.Errorf("IsImageTypeSubtitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizedFFProbeHaveImageTypeSubtitle(t *testing.T) {
	tests := []struct {
		name      string
		subtitles []*Subtitle
		want      bool
	}{
		{
			name: "Has PGS subtitle",
			subtitles: []*Subtitle{
				{Format: "srt"},
				{Format: "pgs"},
			},
			want: true,
		},
		{
			name: "No PGS subtitle",
			subtitles: []*Subtitle{
				{Format: "srt"},
				{Format: "ass"},
			},
			want: false,
		},
		{
			name:      "Empty subtitles",
			subtitles: []*Subtitle{},
			want:      false,
		},
		{
			name:      "Nil subtitles",
			subtitles: nil,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &NormalizedFFProbe{Subtitle: tt.subtitles}
			got := probe.HaveImageTypeSubtitle()
			if got != tt.want {
				t.Errorf("HaveImageTypeSubtitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizedFFProbeGetPGSSubtitles(t *testing.T) {
	tests := []struct {
		name      string
		subtitles []*Subtitle
		wantCount int
	}{
		{
			name: "Multiple PGS subtitles",
			subtitles: []*Subtitle{
				{Id: 1, Format: "pgs", Language: "eng"},
				{Id: 2, Format: "srt", Language: "spa"},
				{Id: 3, Format: "PGS", Language: "fre"},
			},
			wantCount: 2,
		},
		{
			name: "No PGS subtitles",
			subtitles: []*Subtitle{
				{Format: "srt"},
				{Format: "ass"},
			},
			wantCount: 0,
		},
		{
			name:      "Empty subtitles",
			subtitles: []*Subtitle{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &NormalizedFFProbe{Subtitle: tt.subtitles}
			got := probe.GetPGSSubtitles()
			if len(got) != tt.wantCount {
				t.Errorf("GetPGSSubtitles() returned %d subtitles, want %d", len(got), tt.wantCount)
			}

			// Verify all returned subtitles are PGS
			for _, sub := range got {
				if !sub.IsImageTypeSubtitle() {
					t.Errorf("GetPGSSubtitles() returned non-PGS subtitle: %s", sub.Format)
				}
			}
		})
	}
}

func TestNormalizedFFProbeToJson(t *testing.T) {
	probe := &NormalizedFFProbe{
		Video: &Video{
			Id:        0,
			Duration:  time.Hour,
			FrameRate: 30,
		},
		Audios: []*Audio{
			{
				Id:       1,
				Language: "eng",
				Channels: "stereo",
			},
		},
		Subtitle: []*Subtitle{
			{
				Id:       2,
				Language: "spa",
				Format:   "srt",
			},
		},
	}

	json := probe.ToJson()
	if json == "" {
		t.Error("ToJson() returned empty string")
	}

	// Verify it contains expected fields
	if !contains(json, "Video") {
		t.Error("JSON should contain 'Video'")
	}
	if !contains(json, "Audios") {
		t.Error("JSON should contain 'Audios'")
	}
	if !contains(json, "Subtitle") {
		t.Error("JSON should contain 'Subtitle'")
	}
}

func TestVideoStructure(t *testing.T) {
	video := &Video{
		Id:        0,
		Duration:  time.Minute * 90,
		FrameRate: 24,
	}

	if video.Id != 0 {
		t.Errorf("Id = %d, want 0", video.Id)
	}

	if video.Duration != time.Minute*90 {
		t.Errorf("Duration = %v, want %v", video.Duration, time.Minute*90)
	}

	if video.FrameRate != 24 {
		t.Errorf("FrameRate = %d, want 24", video.FrameRate)
	}
}

func TestAudioStructure(t *testing.T) {
	audio := &Audio{
		Id:             1,
		Language:       "eng",
		Channels:       "5.1",
		ChannelsNumber: 6,
		ChannelLayour:  "5.1(side)",
		Default:        true,
		Bitrate:        320,
		Title:          "English Track",
	}

	if audio.Language != "eng" {
		t.Errorf("Language = %s, want eng", audio.Language)
	}

	if audio.ChannelsNumber != 6 {
		t.Errorf("ChannelsNumber = %d, want 6", audio.ChannelsNumber)
	}

	if !audio.Default {
		t.Error("Default should be true")
	}

	if audio.Bitrate != 320 {
		t.Errorf("Bitrate = %d, want 320", audio.Bitrate)
	}
}

func TestSubtitleStructure(t *testing.T) {
	subtitle := &Subtitle{
		Id:       2,
		Language: "spa",
		Forced:   true,
		Comment:  false,
		Format:   "srt",
		Title:    "Spanish Subtitles",
	}

	if subtitle.Language != "spa" {
		t.Errorf("Language = %s, want spa", subtitle.Language)
	}

	if !subtitle.Forced {
		t.Error("Forced should be true")
	}

	if subtitle.Comment {
		t.Error("Comment should be false")
	}

	if subtitle.Format != "srt" {
		t.Errorf("Format = %s, want srt", subtitle.Format)
	}
}

func TestFFProbeFrameRate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      int
		wantError bool
	}{
		{"Standard 24 fps", "24/1", 24, false},
		{"Standard 30 fps", "30/1", 30, false},
		{"Standard 60 fps", "60/1", 60, false},
		{"23.976 fps", "24000/1001", 23, false},
		{"29.97 fps", "30000/1001", 29, false},
		{"Invalid format - no slash", "30", 0, true},
		{"Invalid format - too many parts", "30/1/1", 0, true},
		{"Invalid numerator", "abc/1", 0, true},
		{"Invalid denominator", "30/abc", 0, true},
		{"Empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ffProbeFrameRate(tt.input)

			if tt.wantError {
				if err == nil {
					t.Error("ffProbeFrameRate() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ffProbeFrameRate() unexpected error = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("ffProbeFrameRate() = %d, want %d", got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
