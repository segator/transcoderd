package scheduler

import (
	"errors"
	"testing"
)

func TestErrorVariables(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantText string
	}{
		{"ErrNoJobsAvailable", ErrNoJobsAvailable, "no jobs available to process"},
		{"ErrorJobNotFound", ErrorJobNotFound, "job Not found"},
		{"ErrorStreamNotAllowed", ErrorStreamNotAllowed, "upload not allowed"},
		{"ErrorInvalidStatus", ErrorInvalidStatus, "job invalid status"},
		{"ErrorFileSkipped", ErrorFileSkipped, "path skipped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("%s should not be nil", tt.name)
			}
			if tt.err.Error() != tt.wantText {
				t.Errorf("%s.Error() = %q, want %q", tt.name, tt.err.Error(), tt.wantText)
			}
		})
	}
}
func TestErrorsCanBeComparedWithErrorsIs(t *testing.T) {
	if !errors.Is(ErrNoJobsAvailable, ErrNoJobsAvailable) {
		t.Error("errors.Is should return true for same error")
	}
	if errors.Is(ErrNoJobsAvailable, ErrorJobNotFound) {
		t.Error("errors.Is should return false for different errors")
	}
}
