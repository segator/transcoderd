package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Keep IANA zones available in minimal worker images.
)

// ScheduleConfig restricts new job requests, leaving accepted jobs running.
type ScheduleConfig struct {
	TimeZone string   `mapstructure:"timeZone"`
	Windows  []string `mapstructure:"windows"`
}

type admissionWindow struct {
	days       uint8
	start, end int
}

func (c *Config) validateSchedule() error {
	if c.Schedule == nil {
		return nil
	}
	if len(c.Schedule.Windows) > 0 && c.HaveSettedPeriodTime() {
		return fmt.Errorf("worker.schedule.windows cannot be combined with worker.startAfter or worker.stopAfter")
	}
	_, _, err := c.Schedule.parse()
	return err
}

// AdmissionAllowedAt evaluates the time policy; pause and capacity are separate.
func (c *Config) AdmissionAllowedAt(now time.Time) bool {
	if c.Schedule != nil && len(c.Schedule.Windows) > 0 {
		location, windows, err := c.Schedule.parse()
		if err != nil || c.HaveSettedPeriodTime() {
			return false
		}
		local := now.In(location)
		minute := local.Hour()*60 + local.Minute()
		day := local.Weekday()
		previousDay := (day + 6) % 7
		for _, window := range windows {
			if window.start < window.end {
				if window.includes(day) && minute >= window.start && minute < window.end {
					return true
				}
			} else if (window.includes(day) && minute >= window.start) ||
				(window.includes(previousDay) && minute < window.end) {
				return true
			}
		}
		return false
	}
	if c.HaveSettedPeriodTime() {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		elapsed := now.Sub(midnight)
		return elapsed >= *c.StartAfter && elapsed <= *c.StopAfter
	}
	return true
}

func (w admissionWindow) includes(day time.Weekday) bool {
	return w.days&(1<<uint(day)) != 0
}

func (s *ScheduleConfig) parse() (*time.Location, []admissionWindow, error) {
	zone := s.TimeZone
	if zone == "" {
		zone = "Local"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, nil, fmt.Errorf("worker.schedule.timeZone %q: %w", zone, err)
	}
	windows := make([]admissionWindow, 0, len(s.Windows))
	for _, entry := range s.Windows {
		window, err := parseAdmissionWindow(entry)
		if err != nil {
			return nil, nil, fmt.Errorf("worker.schedule.windows entry %q: %w", entry, err)
		}
		windows = append(windows, window)
	}
	return location, windows, nil
}

func parseAdmissionWindow(entry string) (admissionWindow, error) {
	days, hours, ok := strings.Cut(strings.TrimSpace(entry), "@")
	if !ok {
		days, hours = "*", strings.TrimSpace(entry)
	}
	mask, err := parseAdmissionDays(days)
	if err != nil {
		return admissionWindow{}, err
	}
	startText, endText, ok := strings.Cut(hours, "-")
	if !ok {
		return admissionWindow{}, fmt.Errorf("expected start and end in HH:MM-HH:MM format")
	}
	start, err := parseAdmissionMinute(startText, false)
	if err != nil {
		return admissionWindow{}, err
	}
	end, err := parseAdmissionMinute(endText, true)
	if err != nil {
		return admissionWindow{}, err
	}
	if start == end {
		return admissionWindow{}, fmt.Errorf("start and end must differ; use 00:00-24:00 for a full day")
	}
	return admissionWindow{days: mask, start: start, end: end}, nil
}

func parseAdmissionDays(value string) (uint8, error) {
	if value == "*" {
		return 0x7f, nil
	}
	names := map[string]time.Weekday{
		"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
		"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
	}
	first, last, isRange := strings.Cut(strings.ToLower(value), "-")
	start, ok := names[first]
	if !ok {
		return 0, fmt.Errorf("days must be *, a weekday such as Mon, or a range such as Mon-Fri")
	}
	end := start
	if isRange {
		end, ok = names[last]
		if !ok {
			return 0, fmt.Errorf("invalid weekday range %q", value)
		}
	}
	var mask uint8
	for day := start; ; day = (day + 1) % 7 {
		mask |= 1 << uint(day)
		if day == end {
			return mask, nil
		}
	}
}

func parseAdmissionMinute(value string, allowEndOfDay bool) (int, error) {
	if allowEndOfDay && value == "24:00" {
		return 24 * 60, nil
	}
	if len(value) != 5 || value[2] != ':' || value[0] < '0' || value[0] > '9' || value[3] < '0' || value[3] > '9' {
		return 0, fmt.Errorf("invalid clock time %q; expected HH:MM", value)
	}
	hour, hourErr := strconv.Atoi(value[:2])
	minute, minuteErr := strconv.Atoi(value[3:])
	if hourErr != nil || minuteErr != nil || hour > 23 || minute > 59 {
		return 0, fmt.Errorf("invalid clock time %q", value)
	}
	return hour*60 + minute, nil
}
