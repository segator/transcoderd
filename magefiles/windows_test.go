package main

import "testing"

func TestWindowsCompilePlan(t *testing.T) {
	tests := []struct {
		input                  string
		budget, parallel, jobs int
		wantErr                bool
	}{
		{input: "", budget: 16, parallel: 4, jobs: 4},
		{input: "1", budget: 1, parallel: 1, jobs: 1},
		{input: "2", budget: 2, parallel: 2, jobs: 1},
		{input: "7", budget: 7, parallel: 4, jobs: 1},
		{input: "16", budget: 16, parallel: 4, jobs: 4},
		{input: "32", budget: 32, parallel: 4, jobs: 8},
		{input: "0", wantErr: true},
		{input: "-2", wantErr: true},
		{input: "auto", wantErr: true},
		{input: "2; nproc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			budget, parallel, jobs, err := windowsCompilePlan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("windowsCompilePlan(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if budget != tt.budget || parallel != tt.parallel || jobs != tt.jobs {
				t.Errorf("windowsCompilePlan(%q) = (%d, %d, %d), want (%d, %d, %d)", tt.input, budget, parallel, jobs, tt.budget, tt.parallel, tt.jobs)
			}
			if !tt.wantErr && (parallel*jobs > budget || jobs < 1) {
				t.Errorf("windowsCompilePlan(%q) schedules %d compiler jobs for budget %d", tt.input, parallel*jobs, budget)
			}
		})
	}
}
