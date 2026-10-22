package config

import (
	"testing"
	"time"
)

func TestWeeklyAdmission(t *testing.T) {
	windows := []string{"Mon-Fri@00:00-10:00", "Mon-Fri@14:00-18:00", "Mon-Fri@22:00-24:00", "Sat-Sun@00:00-24:00"}
	tests := []struct {
		name string
		at   string
		want bool
	}{
		{"start inclusive", "2026-10-19T00:00:00+02:00", true},
		{"last second before cutoff", "2026-10-19T09:59:59+02:00", true},
		{"end exclusive", "2026-10-19T10:00:00+02:00", false},
		{"closed period", "2026-10-19T13:59:59+02:00", false},
		{"second window opens", "2026-10-19T14:00:00+02:00", true},
		{"second window closes", "2026-10-19T18:00:00+02:00", false},
		{"evening opens", "2026-10-19T22:00:00+02:00", true},
		{"end of day", "2026-10-19T23:59:59+02:00", true},
		{"Saturday afternoon", "2026-10-24T12:00:00+02:00", true},
		{"Sunday afternoon", "2026-10-25T12:00:00+01:00", true},
		{"UTC input maps to local cutoff", "2026-10-19T08:00:00Z", false},
		{"winter offset maps to local cutoff", "2026-11-02T09:00:00Z", false},
	}
	c := Config{Schedule: &ScheduleConfig{TimeZone: "Europe/Madrid", Windows: windows}}
	if err := c.validateSchedule(); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.AdmissionAllowedAt(at); got != tt.want {
				t.Errorf("AdmissionAllowedAt(%s) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestWeeklyAdmissionOvernightAndDST(t *testing.T) {
	tests := []struct {
		name, window, at string
		want             bool
	}{
		{"before overnight start", "Fri@22:00-08:00", "2026-10-23T21:59:59+02:00", false},
		{"overnight start", "Fri@22:00-08:00", "2026-10-23T22:00:00+02:00", true},
		{"next day tail", "Fri@22:00-08:00", "2026-10-24T07:59:59+02:00", true},
		{"tail end exclusive", "Fri@22:00-08:00", "2026-10-24T08:00:00+02:00", false},
		{"tail requires previous day", "Fri@22:00-08:00", "2026-10-23T07:00:00+02:00", false},
		{"week wraps", "Sun@22:00-08:00", "2026-10-26T07:00:00+01:00", true},
		{"range wraps", "Fri-Mon@12:00-14:00", "2026-10-25T12:00:00+01:00", true},
		{"case insensitive", "mon@12:00-14:00", "2026-10-19T12:00:00+02:00", true},
		{"all days", "*@12:00-14:00", "2026-10-21T12:00:00+02:00", true},
		{"daily shorthand opens", "22:00-10:00", "2026-10-19T22:00:00+02:00", true},
		{"daily shorthand overnight", "22:00-10:00", "2026-10-20T09:59:59+02:00", true},
		{"daily shorthand closes", "22:00-10:00", "2026-10-20T10:00:00+02:00", false},
		{"daily shorthand closed", "22:00-10:00", "2026-10-20T21:59:59+02:00", false},
		{"spring uses civil clock", "Sun@03:00-04:00", "2026-03-29T01:30:00Z", true},
		{"spring nonexistent hour skipped", "Sun@02:00-03:00", "2026-03-29T01:30:00Z", false},
		{"autumn first repeated hour", "Sun@02:00-03:00", "2026-10-25T00:30:00Z", true},
		{"autumn second repeated hour", "Sun@02:00-03:00", "2026-10-25T01:30:00Z", true},
		{"autumn end", "Sun@02:00-03:00", "2026-10-25T02:00:00Z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Schedule: &ScheduleConfig{TimeZone: "Europe/Madrid", Windows: []string{tt.window}}}
			if err := c.validateSchedule(); err != nil {
				t.Fatal(err)
			}
			at, err := time.Parse(time.RFC3339, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.AdmissionAllowedAt(at); got != tt.want {
				t.Errorf("AdmissionAllowedAt(%s) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestScheduleValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  ScheduleConfig
	}{
		{"invalid zone", ScheduleConfig{TimeZone: "Not/A_Zone"}},
		{"missing hours", ScheduleConfig{Windows: []string{"Mon-Fri"}}},
		{"unknown day", ScheduleConfig{Windows: []string{"Monday@00:00-10:00"}}},
		{"invalid range", ScheduleConfig{Windows: []string{"Mon-Bad@00:00-10:00"}}},
		{"missing end", ScheduleConfig{Windows: []string{"*@00:00"}}},
		{"invalid clock", ScheduleConfig{Windows: []string{"*@9:00-10:00"}}},
		{"hour overflow", ScheduleConfig{Windows: []string{"*@00:00-25:00"}}},
		{"minute overflow", ScheduleConfig{Windows: []string{"*@00:00-10:60"}}},
		{"negative minute", ScheduleConfig{Windows: []string{"*@00:00-10:-1"}}},
		{"end of day as start", ScheduleConfig{Windows: []string{"*@24:00-10:00"}}},
		{"empty interval", ScheduleConfig{Windows: []string{"*@10:00-10:00"}}},
		{"blank entry", ScheduleConfig{Windows: []string{""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{MaxActiveJobs: 2, EncodeConfig: &FFMPEGConfig{ParallelJobs: 2}, Schedule: &tt.cfg}
			if err := c.Validate(); err == nil {
				t.Error("Validate() error = nil, want schedule error")
			}
		})
	}
	start, stop := time.Duration(0), 10*time.Hour
	c := Config{StartAfter: &start, StopAfter: &stop, Schedule: &ScheduleConfig{Windows: []string{"*@00:00-10:00"}}}
	if err := c.validateSchedule(); err == nil {
		t.Error("validateSchedule() error = nil, want conflict error")
	}
	if c.AdmissionAllowedAt(time.Now()) {
		t.Error("AdmissionAllowedAt() = true for conflicting settings, want false")
	}
	c.Schedule.Windows = nil
	if err := c.validateSchedule(); err != nil {
		t.Errorf("validateSchedule() with no weekly windows error = %v", err)
	}
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	if !c.AdmissionAllowedAt(at) || c.AdmissionAllowedAt(at.Add(2*time.Hour)) {
		t.Error("legacy interval changed when weekly windows are empty")
	}
	if got := (&Config{}).AdmissionAllowedAt(at); !got {
		t.Error("AdmissionAllowedAt() with no time policy = false, want true")
	}
}
