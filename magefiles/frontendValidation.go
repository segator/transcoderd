package main

import (
	"archive/tar"
	"context"
	"crypto/x509"
	"debug/elf"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dagger.io/dagger"
)

func checkFrontendImages(ctx context.Context, client *dagger.Client, images []*dagger.Container) error {
	if len(images) != len(frontendPlatforms) {
		return fmt.Errorf("frontend variants = %d, want %d", len(images), len(frontendPlatforms))
	}
	dir, err := os.MkdirTemp("", "transcoderd-frontend-platforms-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	httpClient := client.Container().From("debian:trixie-slim").
		WithExec([]string{"sh", "-c", "apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*"})
	for _, image := range images {
		platform, err := image.Platform(ctx)
		if err != nil {
			return err
		}
		binaryPath := filepath.Join(dir, strings.ReplaceAll(string(platform), "/", "-"))
		if _, err := image.File("/app/transcoderd-frontend").Export(ctx, binaryPath); err != nil {
			return err
		}
		if err := checkFrontendELF(binaryPath, string(platform)); err != nil {
			return err
		}
		user, err := image.User(ctx)
		if err != nil || user != "65532:65532" {
			return fmt.Errorf("frontend %s must use nonroot UID/GID 65532 (got %q, error %v)", platform, user, err)
		}
		certificates, err := image.File("/etc/ssl/certs/ca-certificates.crt").Contents(ctx)
		if err != nil || !x509.NewCertPool().AppendCertsFromPEM([]byte(certificates)) {
			return fmt.Errorf("frontend %s is missing its CA certificate bundle (error %v)", platform, err)
		}
		version, err := image.WithExec([]string{"/app/transcoderd-frontend", "--version"}).Stderr(ctx)
		if err != nil {
			return fmt.Errorf("run frontend %s: %w", platform, err)
		}
		if !strings.Contains(version, projectVersion) {
			return fmt.Errorf("frontend %s version = %q, want application version %q", platform, version, projectVersion)
		}
		status, err := httpClient.WithServiceBinding("frontend", image.AsService()).
			WithExec([]string{"curl", "--fail", "--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}", "http://frontend:8081/healthz"}).Stdout(ctx)
		if err != nil {
			return fmt.Errorf("frontend %s health check: %w", platform, err)
		}
		if status != "204" {
			return fmt.Errorf("frontend %s health status = %q, want 204", platform, status)
		}
		fmt.Printf("Validated frontend %s: static ELF, version, nonroot runtime and HTTP health\n", platform)
	}
	archive := filepath.Join(dir, "frontend-image.tar")
	if _, err := images[0].AsTarball(dagger.ContainerAsTarballOpts{PlatformVariants: images[1:]}).Export(ctx, archive); err != nil {
		return err
	}
	return checkFrontendIndex(archive)
}

func checkFrontendELF(path, platform string) error {
	binary, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open frontend %s ELF: %w", platform, err)
	}
	defer binary.Close()
	want := map[string]elf.Machine{"linux/amd64": elf.EM_X86_64, "linux/arm64": elf.EM_AARCH64}[platform]
	if want == elf.EM_NONE || binary.Machine != want || binary.Class != elf.ELFCLASS64 {
		return fmt.Errorf("frontend %s ELF machine = %s/%s, want %s/ELF64", platform, binary.Machine, binary.Class, want)
	}
	for _, program := range binary.Progs {
		if program.Type == elf.PT_INTERP {
			return fmt.Errorf("frontend %s must be statically linked", platform)
		}
	}
	return nil
}

type imageDescriptor struct {
	Digest   string `json:"digest"`
	Platform *struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform"`
}

type imageDocument struct {
	Manifests    []imageDescriptor `json:"manifests"`
	Config       imageDescriptor   `json:"config"`
	OS           string            `json:"os"`
	Architecture string            `json:"architecture"`
}

func checkFrontendIndex(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	documents := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Image/index/config JSON is small; avoid retaining compressed layers.
		if header.Size > 1<<20 || header.Typeflag != tar.TypeReg {
			continue
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		documents[strings.TrimPrefix(header.Name, "./")] = content
	}
	load := func(name string) (imageDocument, error) {
		var document imageDocument
		if err := json.Unmarshal(documents[name], &document); err != nil {
			return document, fmt.Errorf("read OCI document %s: %w", name, err)
		}
		return document, nil
	}
	index, err := load("index.json")
	if err != nil {
		return err
	}
	platforms := map[string]bool{"linux/amd64": false, "linux/arm64": false}
	visited := map[string]bool{}
	var visit func(imageDescriptor) error
	visit = func(descriptor imageDescriptor) error {
		if !strings.HasPrefix(descriptor.Digest, "sha256:") || visited[descriptor.Digest] {
			return fmt.Errorf("invalid or repeated OCI descriptor %q", descriptor.Digest)
		}
		visited[descriptor.Digest] = true
		document, err := load("blobs/sha256/" + strings.TrimPrefix(descriptor.Digest, "sha256:"))
		if err != nil {
			return err
		}
		if len(document.Manifests) != 0 {
			for _, child := range document.Manifests {
				if err := visit(child); err != nil {
					return err
				}
			}
			return nil
		}
		config, err := load("blobs/sha256/" + strings.TrimPrefix(document.Config.Digest, "sha256:"))
		if err != nil {
			return err
		}
		platform := config.OS + "/" + config.Architecture
		seen, supported := platforms[platform]
		if !supported || seen || descriptor.Platform == nil || descriptor.Platform.OS+"/"+descriptor.Platform.Architecture != platform {
			return fmt.Errorf("unexpected, duplicate or mismatched frontend platform %q", platform)
		}
		platforms[platform] = true
		return nil
	}
	for _, descriptor := range index.Manifests {
		if err := visit(descriptor); err != nil {
			return err
		}
	}
	for platform, found := range platforms {
		if !found {
			return fmt.Errorf("frontend OCI index is missing %s", platform)
		}
	}
	return nil
}
