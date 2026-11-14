package main

import (
	"context"
	"strings"

	"dagger.io/dagger"
)

var frontendPlatforms = []dagger.Platform{"linux/amd64", "linux/arm64"}

func frontendImage(client *dagger.Client, src *dagger.Directory, platform dagger.Platform) *dagger.Container {
	arch := strings.TrimPrefix(string(platform), "linux/")
	binary := applicationBinary(client, src, "frontend", "linux", arch)
	// CA data is architecture independent. Provision it once on the native
	// builder; constructing either runtime needs no foreign package execution.
	certificates := client.Container().From("debian:trixie-slim").
		WithExec([]string{"sh", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*"}).
		File("/etc/ssl/certs/ca-certificates.crt")
	return client.Container(dagger.ContainerOpts{Platform: platform}).From("debian:trixie-slim").
		WithFile("/etc/ssl/certs/ca-certificates.crt", certificates).
		WithFile("/app/transcoderd-frontend", binary).
		WithUser("65532:65532").WithExposedPort(8081).
		WithEntrypoint([]string{"/app/transcoderd-frontend"})
}

func applicationImages(ctx context.Context, client *dagger.Client, src *dagger.Directory, component string) ([]*dagger.Container, error) {
	image, err := containerImage(ctx, client, src, component)
	if err != nil {
		return nil, err
	}
	images := []*dagger.Container{image}
	if component == "frontend" {
		for _, platform := range frontendPlatforms[1:] {
			images = append(images, frontendImage(client, src, platform))
		}
	}
	return images, nil
}

// FrontendImages verifies both frontend runtimes and their shared OCI index.
func (Test) FrontendImages(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	images, err := applicationImages(ctx, client, projectSource(client), "frontend")
	if err != nil {
		return err
	}
	return checkFrontendImages(ctx, client, images)
}
