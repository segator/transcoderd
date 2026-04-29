package ffmpeg

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	ffprobe "gopkg.in/vansante/go-ffprobe.v2"
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

func TestSubtitleNeedsMKVExtraction(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{"Empty format (ffprobe unknown codec)", "", true},
		{"None format (ffprobe unrecognized)", "none", true},
		{"mov_text (MP4-only codec)", "mov_text", true},
		{"SRT subtitle", "srt", false},
		{"Subrip subtitle", "subrip", false},
		{"ASS subtitle", "ass", false},
		{"SSA subtitle", "ssa", false},
		{"PGS subtitle (handled by OCR pipeline)", "hdmv_pgs_subtitle", false},
		{"PGS uppercase", "PGS", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &Subtitle{Format: tt.format}
			got := sub.NeedsMKVExtraction()
			if got != tt.want {
				t.Errorf("NeedsMKVExtraction() = %v, want %v for format %q", got, tt.want, tt.format)
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

func TestNormalizedFFProbeGetExtractableSubtitles(t *testing.T) {
	tests := []struct {
		name      string
		subtitles []*Subtitle
		wantCount int
	}{
		{
			name: "Has WebVTT-like (empty codec) subtitles",
			subtitles: []*Subtitle{
				{Id: 1, Format: "srt", Language: "eng"},
				{Id: 2, Format: "", Language: "spa"},
				{Id: 3, Format: "none", Language: "fre"},
			},
			wantCount: 2,
		},
		{
			name: "Has mov_text subtitle",
			subtitles: []*Subtitle{
				{Id: 1, Format: "srt", Language: "eng"},
				{Id: 2, Format: "mov_text", Language: "spa"},
			},
			wantCount: 1,
		},
		{
			name: "PGS is not extractable (handled by OCR pipeline)",
			subtitles: []*Subtitle{
				{Id: 1, Format: "pgs", Language: "eng"},
				{Id: 2, Format: "", Language: "spa"},
			},
			wantCount: 1,
		},
		{
			name: "No extractable subtitles",
			subtitles: []*Subtitle{
				{Id: 1, Format: "srt", Language: "eng"},
				{Id: 2, Format: "ass", Language: "spa"},
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
			got := probe.GetExtractableSubtitles()
			if len(got) != tt.wantCount {
				t.Errorf("GetExtractableSubtitles() returned %d subtitles, want %d", len(got), tt.wantCount)
			}
			for _, sub := range got {
				if !sub.NeedsMKVExtraction() {
					t.Errorf("GetExtractableSubtitles() returned non-extractable subtitle: %q", sub.Format)
				}
			}
		})
	}
}

func TestNormalizedFFProbeHaveExtractableSubtitle(t *testing.T) {
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
			name: "Has unsupported codec subtitle",
			subtitles: []*Subtitle{
				{Format: "srt"},
				{Format: ""},
			},
			want: true,
		},
		{
			name: "Has both PGS and unsupported codec",
			subtitles: []*Subtitle{
				{Format: "pgs"},
				{Format: "none"},
			},
			want: true,
		},
		{
			name: "Only normal text subtitles",
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
			got := probe.HaveExtractableSubtitle()
			if got != tt.want {
				t.Errorf("HaveExtractableSubtitle() = %v, want %v", got, tt.want)
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

func TestNormalizeFFProbeData_DualAudio(t *testing.T) {
	rawJSON := `{
		"streams": [
			{
				"index": 0,
				"codec_type": "video",
				"avg_frame_rate": "24/1",
				"codec_name": "h264"
			},
			{
				"index": 1,
				"codec_type": "audio",
				"bit_rate": "384000",
				"channels": 6,
				"channel_layout": "5.1",
				"tags": {"language": "spa", "title": "Spanish"},
				"disposition": {"default": 1, "forced": 0, "comment": 0}
			},
			{
				"index": 2,
				"codec_type": "audio",
				"bit_rate": "768000",
				"channels": 6,
				"channel_layout": "5.1",
				"tags": {"language": "eng", "title": "English"},
				"disposition": {"default": 0, "forced": 0, "comment": 0}
			},
			{
				"index": 3,
				"codec_type": "subtitle",
				"codec_name": "subrip",
				"tags": {"language": "spa", "title": "Spanish"},
				"disposition": {"default": 0, "forced": 0, "comment": 0}
			},
			{
				"index": 4,
				"codec_type": "subtitle",
				"codec_name": "subrip",
				"tags": {"language": "eng", "title": "English"},
				"disposition": {"default": 0, "forced": 0, "comment": 0}
			}
		],
		"format": {
			"duration": "600.000000"
		}
	}`

	var probeData ffprobe.ProbeData
	if err := json.Unmarshal([]byte(rawJSON), &probeData); err != nil {
		t.Fatalf("Failed to unmarshal probe data: %v", err)
	}

	result, err := NormalizeFFProbeData(&probeData)
	if err != nil {
		t.Fatalf("NormalizeFFProbeData() error = %v", err)
	}

	if len(result.Audios) != 2 {
		t.Fatalf("NormalizeFFProbeData() got %d audio tracks, want 2", len(result.Audios))
	}

	audioLangs := make(map[string]bool)
	for _, a := range result.Audios {
		audioLangs[a.Language] = true
	}
	if !audioLangs["spa"] {
		t.Error("NormalizeFFProbeData() missing Spanish audio track")
	}
	if !audioLangs["eng"] {
		t.Error("NormalizeFFProbeData() missing English audio track")
	}

	if len(result.Subtitle) != 2 {
		t.Fatalf("NormalizeFFProbeData() got %d subtitle tracks, want 2", len(result.Subtitle))
	}

	subLangs := make(map[string]bool)
	for _, s := range result.Subtitle {
		subLangs[s.Language] = true
	}
	if !subLangs["spa"] {
		t.Error("NormalizeFFProbeData() missing Spanish subtitle track")
	}
	if !subLangs["eng"] {
		t.Error("NormalizeFFProbeData() missing English subtitle track")
	}
}

func TestNormalizeFFProbeData_SameLanguagePrefersHigherBitrate(t *testing.T) {
	rawJSON := `{
		"streams": [
			{
				"index": 0,
				"codec_type": "video",
				"avg_frame_rate": "24/1",
				"codec_name": "h264"
			},
			{
				"index": 1,
				"codec_type": "audio",
				"bit_rate": "384000",
				"channels": 6,
				"channel_layout": "5.1",
				"tags": {"language": "eng", "title": "English AC3"},
				"disposition": {"default": 1, "forced": 0, "comment": 0}
			},
			{
				"index": 2,
				"codec_type": "audio",
				"bit_rate": "768000",
				"channels": 6,
				"channel_layout": "5.1",
				"tags": {"language": "eng", "title": "English DTS"},
				"disposition": {"default": 0, "forced": 0, "comment": 0}
			}
		],
		"format": {
			"duration": "600.000000"
		}
	}`

	var probeData ffprobe.ProbeData
	if err := json.Unmarshal([]byte(rawJSON), &probeData); err != nil {
		t.Fatalf("Failed to unmarshal probe data: %v", err)
	}

	result, err := NormalizeFFProbeData(&probeData)
	if err != nil {
		t.Fatalf("NormalizeFFProbeData() error = %v", err)
	}

	if len(result.Audios) != 1 {
		t.Fatalf("NormalizeFFProbeData() got %d audio tracks, want 1 (same language dedup)", len(result.Audios))
	}
	if result.Audios[0].Bitrate != 768000 {
		t.Errorf("NormalizeFFProbeData() kept bitrate %d, want 768000 (higher)", result.Audios[0].Bitrate)
	}
	if result.Audios[0].Title != "English DTS" {
		t.Errorf("NormalizeFFProbeData() kept title %q, want \"English DTS\"", result.Audios[0].Title)
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
