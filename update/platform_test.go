package update

import "testing"

func TestPlatformAssetName(t *testing.T) {
	tests := []struct{ name, goos, goarch, want string }{
		{"legacy linux", "linux", "amd64", "transcoderd-worker"},
		{"windows x64", "windows", "amd64", "transcoderd-worker-windows-amd64.exe"},
		{"linux arm", "linux", "arm64", "transcoderd-worker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := platformAssetName("transcoderd-worker", tt.goos, tt.goarch); got != tt.want {
				t.Errorf("platformAssetName() = %q, want %q", got, tt.want)
			}
		})
	}
}
