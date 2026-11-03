package config

import "testing"

func TestPauseModeValidation(t *testing.T) {
	for _, tt := range []struct {
		value, want string
		valid       bool
	}{{"", "finish", true}, {"finish", "finish", true}, {"ram", "ram", true}, {"RAM pause", "", false}, {"chunked", "", false}} {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ResolvePauseMode(tt.value)
			if (err == nil) != tt.valid || got != tt.want {
				t.Fatalf("ResolvePauseMode(%q) = %q, %v; want %q valid %t", tt.value, got, err, tt.want, tt.valid)
			}
		})
	}
}
