package model

import (
	"fmt"
	"time"
)

// DashboardRange uses a start-inclusive and end-exclusive UTC interval.
type DashboardRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func ResolveDashboardRange(hours int, now time.Time, ranges ...DashboardRange) (DashboardRange, error) {
	if len(ranges) > 1 {
		return DashboardRange{}, fmt.Errorf("only one reporting range is allowed")
	}
	if len(ranges) == 1 {
		r := ranges[0]
		if r.Start.IsZero() || r.End.IsZero() || !r.End.After(r.Start) || r.End.Sub(r.Start) > 3660*24*time.Hour {
			return DashboardRange{}, fmt.Errorf("range must have start before end and span at most 3660 days")
		}
		return DashboardRange{Start: r.Start.UTC(), End: r.End.UTC()}, nil
	}
	if hours != 24 && hours != 168 && hours != 720 {
		return DashboardRange{}, fmt.Errorf("dashboard hours must be 24, 168, or 720")
	}
	return DashboardRange{Start: now.UTC().Add(-time.Duration(hours) * time.Hour), End: now.UTC()}, nil
}

func (r DashboardRange) Contains(at time.Time) bool { return !at.Before(r.Start) && at.Before(r.End) }
func (r DashboardRange) Step() time.Duration {
	if r.End.Sub(r.Start) <= 24*time.Hour {
		return time.Hour
	}
	return 24 * time.Hour
}
