package main

import (
	"context"
	"fmt"
	"path/filepath"

	"dagger.io/dagger"
)

// SVTComparison exports matched production FFmpeg builds differing only in SVT.
// Other dependencies reuse the existing builder's content-addressed cache.
func (Build) SVTComparison(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	for _, version := range []string{"4.2.0", "4.3.0"} {
		builder := rebuildFFmpegSVT(client, version)
		output := client.Directory().
			WithDirectory("bin", builder.Directory("/app/ffmpeg-build-script/workspace/bin")).
			WithFile("toolchain.json", builder.File("/app/ffmpeg-build-script/workspace/svt-toolchain.json"))
		if _, err := output.Export(ctx, filepath.Join(projectRoot(), "build", "av1-lab", "svt-4-3", "tools", version)); err != nil {
			return fmt.Errorf("export SVT %s: %w", version, err)
		}
	}
	return nil
}

// SVTWindowsTools exports the candidate's native Windows media toolchain.
func (Build) SVTWindowsTools(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = windowsFFmpeg(client).Directory("/output").Export(ctx,
		filepath.Join(projectRoot(), "build", "av1-lab", "svt-4-3", "windows-tools"))
	return err
}

func rebuildFFmpegSVT(client *dagger.Client, version string) *dagger.Container {
	return buildFFmpegBaselineImage(client).
		WithFile("/svt-toolchain.py", client.Host().File(filepath.Join(projectRoot(), "scripts", "ffmpeg", "svt-toolchain.py"))).
		WithExec([]string{"python3", "/svt-toolchain.py", version}).
		WithFile("/verify-svt.sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "ffmpeg", "verify-svt.sh"))).
		WithExec([]string{"bash", "/verify-svt.sh", "/app/ffmpeg-build-script/workspace/bin", version}).
		WithoutDirectory("/svt-rebuild")
}
