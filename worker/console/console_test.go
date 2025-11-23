package console

import (
	"testing"
)

func TestFormatNumber(t *testing.T) {
	notations := map[int64]string{
		1000000000000000: "PB",
		1000000000000:    "TB",
		1000000000:       "GB",
		1000000:          "MB",
		1000:             "KB",
		0:                "B",
	}
	tests := []struct {
		name  string
		value int64
		want  string
	}{
		{"Bytes", 500, "500B"},
		{"Kilobytes", 1500, "1.50KB"},
		{"Megabytes", 1500000, "1.50MB"},
		{"Gigabytes", 1500000000, "1.50GB"},
		{"Zero", 0, "0B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatNumber(tt.value, notations)
			if got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
func TestUnitScales(t *testing.T) {
	if len(unitScales) == 0 {
		t.Error("unitScales should not be empty")
	}
	for i := 0; i < len(unitScales)-1; i++ {
		if unitScales[i] <= unitScales[i+1] {
			t.Errorf("unitScales not in descending order at index %d", i)
		}
	}
}
