package command

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := NewCommand("echo", "hello", "world")

	if cmd.Command != "echo" {
		t.Errorf("Command = %s, want echo", cmd.Command)
	}

	if len(cmd.Params) != 2 {
		t.Errorf("len(Params) = %d, want 2", len(cmd.Params))
	}

	if cmd.Params[0] != "hello" || cmd.Params[1] != "world" {
		t.Errorf("Params = %v, want [hello world]", cmd.Params)
	}

	if cmd.buffSize != 4096 {
		t.Errorf("buffSize = %d, want 4096", cmd.buffSize)
	}
}

func TestNewCommandByString(t *testing.T) {
	cmd := NewCommandByString("echo", "hello world test")

	if cmd.Command != "echo" {
		t.Errorf("Command = %s, want echo", cmd.Command)
	}

	if len(cmd.Params) != 3 {
		t.Errorf("len(Params) = %d, want 3", len(cmd.Params))
	}
}

func TestCommandAddParam(t *testing.T) {
	cmd := NewCommand("ls")
	cmd.AddParam("-la")
	cmd.AddParam("/tmp")

	if len(cmd.Params) != 2 {
		t.Errorf("len(Params) = %d, want 2", len(cmd.Params))
	}

	if cmd.Params[0] != "-la" {
		t.Errorf("Params[0] = %s, want -la", cmd.Params[0])
	}
}

func TestCommandSetWorkDir(t *testing.T) {
	cmd := NewCommand("pwd")
	cmd.SetWorkDir("/tmp")

	if cmd.WorkDir != "/tmp" {
		t.Errorf("WorkDir = %s, want /tmp", cmd.WorkDir)
	}
}

func TestCommandSetEnv(t *testing.T) {
	cmd := NewCommand("env")
	customEnv := []string{"KEY=value", "FOO=bar"}
	cmd.SetEnv(customEnv)

	if len(cmd.Env) != 2 {
		t.Errorf("len(Env) = %d, want 2", len(cmd.Env))
	}
}

func TestCommandAddEnv(t *testing.T) {
	cmd := NewCommand("env")
	originalLen := len(cmd.Env)

	cmd.AddEnv("CUSTOM_VAR=test")

	if len(cmd.Env) != originalLen+1 {
		t.Errorf("len(Env) = %d, want %d", len(cmd.Env), originalLen+1)
	}

	found := false
	for _, env := range cmd.Env {
		if env == "CUSTOM_VAR=test" {
			found = true
			break
		}
	}

	if !found {
		t.Error("CUSTOM_VAR=test not found in Env")
	}
}

func TestCommandBuffSize(t *testing.T) {
	cmd := NewCommand("cat")
	cmd.BuffSize(8192)

	if cmd.buffSize != 8192 {
		t.Errorf("buffSize = %d, want 8192", cmd.buffSize)
	}
}

func TestCommandSetStdoutFunc(t *testing.T) {
	cmd := NewCommand("echo")
	called := false

	stdoutFunc := func(buffer []byte, exit bool) {
		called = true
	}

	cmd.SetStdoutFunc(stdoutFunc)

	if cmd.stdoutFunc == nil {
		t.Error("stdoutFunc should not be nil")
	}

	// Test that the function works
	cmd.stdoutFunc([]byte("test"), false)
	if !called {
		t.Error("stdoutFunc was not called")
	}
}

func TestCommandSetStderrFunc(t *testing.T) {
	cmd := NewCommand("echo")
	called := false

	stderrFunc := func(buffer []byte, exit bool) {
		called = true
	}

	cmd.SetStderrFunc(stderrFunc)

	if cmd.stderrFunc == nil {
		t.Error("stderrFunc should not be nil")
	}

	// Test that the function works
	cmd.stderrFunc([]byte("test"), false)
	if !called {
		t.Error("stderrFunc was not called")
	}
}

func TestStringToSlice(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "Simple string",
			input: "one two three",
			want:  3,
		},
		{
			name:  "Empty string",
			input: "",
			want:  1, // StringToSlice always appends the last word even if empty
		},
		{
			name:  "Single word",
			input: "word",
			want:  1,
		},
		{
			name:  "String with quotes",
			input: "'hello world' test",
			want:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringToSlice(tt.input)
			if len(result) != tt.want {
				t.Errorf("StringToSlice(%q) returned %d elements, want %d", tt.input, len(result), tt.want)
			}
		})
	}
}

func TestGetWD(t *testing.T) {
	wd := GetWD()

	if wd == "" {
		t.Error("GetWD() returned empty string")
	}

	// Verify it's a valid directory
	if _, err := os.Stat(wd); err != nil {
		t.Errorf("GetWD() returned invalid path: %v", err)
	}
}

func TestNewAllowedCodesOption(t *testing.T) {
	opt := NewAllowedCodesOption(0, 1, 2)

	if len(opt.AllowedCodes) != 3 {
		t.Errorf("len(AllowedCodes) = %d, want 3", len(opt.AllowedCodes))
	}

	expected := []int{0, 1, 2}
	for i, code := range opt.AllowedCodes {
		if code != expected[i] {
			t.Errorf("AllowedCodes[%d] = %d, want %d", i, code, expected[i])
		}
	}
}

// Integration test - only runs actual commands that are safe
func TestCommandExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var output []byte
	stdoutFunc := func(buffer []byte, exit bool) {
		output = append(output, buffer...)
	}

	// Use a command that works on all platforms
	var cmd *Command
	if runtime.GOOS == "windows" {
		cmd = NewCommand("cmd", "/c", "echo", "test")
	} else {
		cmd = NewCommand("echo", "test")
	}

	cmd.SetStdoutFunc(stdoutFunc)

	exitCode, err := cmd.RunWithContext(context.Background())
	if err != nil {
		t.Fatalf("RunWithContext() error = %v", err)
	}

	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}

	if len(output) == 0 {
		t.Error("No output captured")
	}

	outputStr := strings.TrimSpace(string(output))
	if !strings.Contains(outputStr, "test") {
		t.Errorf("output = %q, want to contain 'test'", outputStr)
	}
}

func TestCommandChaining(t *testing.T) {
	cmd := NewCommand("test").
		AddParam("-f").
		AddParam("/tmp").
		SetWorkDir("/").
		BuffSize(2048).
		AddEnv("TEST=1")

	if cmd.Command != "test" {
		t.Error("Command chaining failed")
	}

	if len(cmd.Params) != 2 {
		t.Error("AddParam chaining failed")
	}

	if cmd.WorkDir != "/" {
		t.Error("SetWorkDir chaining failed")
	}

	if cmd.buffSize != 2048 {
		t.Error("BuffSize chaining failed")
	}
}
