package main

import (
	"reflect"
	"testing"
)

func TestParseImageVersionAliases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "trims and removes empty and duplicate aliases",
			input: " v2.5, ,v2,v2.5, ",
			want:  []string{"v2.5", "v2"},
		},
		{
			name:  "empty input",
			input: " , ",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageVersionAliases(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseImageVersionAliases(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestImageTagsFor(t *testing.T) {
	for _, component := range []string{"server", "worker"} {
		t.Run(component, func(t *testing.T) {
			got := imageTagsFor(
				"ghcr.io/segator/transcoderd",
				component,
				"v2.5.1",
				"main",
				[]string{" v2.5 ", "", "v2", "v2.5.1", "main"},
			)
			want := []string{
				"ghcr.io/segator/transcoderd:" + component + "-v2.5.1",
				"ghcr.io/segator/transcoderd:" + component + "-main",
				"ghcr.io/segator/transcoderd:" + component + "-v2.5",
				"ghcr.io/segator/transcoderd:" + component + "-v2",
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("imageTagsFor() = %v, want %v", got, want)
			}
		})
	}
}
