// Package main provides the Mage build targets for TranscoderD.
// All build, test, and lint operations run inside Dagger containers for
// reproducibility and content-addressed caching. Mage is the CLI orchestrator.
//
// Usage:
//
//	mage build         # Build server + worker Go binaries
//	mage test          # Run unit tests with coverage
//	mage testE2E       # Run Docker e2e tests
//	mage lint          # Run golangci-lint
//	mage container     # Build Docker images via Dagger
//	mage publish       # Build and push Docker images via Dagger
//	mage -l            # List all targets
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"dagger.io/dagger"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

var (
	imageName      = envOrDefault("IMAGE_NAME", "ghcr.io/segator/transcoderd")
	projectVersion = ""
	gitCommitSHA   = ""
	gitBranchName  = ""
	buildDate      = ""
)

func init() {
	projectVersion = envOrDefault("PROJECT_VERSION", readVersionFile()+"-dev")
	gitCommitSHA = gitOutput("rev-parse", "--short", "HEAD")
	gitBranchName = sanitizeBranch(gitOutput("rev-parse", "--abbrev-ref", "HEAD"))
	buildDate = time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func readVersionFile() string {
	b, err := os.ReadFile(filepath.Join(projectRoot(), "version.txt"))
	if err != nil {
		return "0.0.0"
	}
	return strings.TrimSpace(string(b))
}

func projectRoot() string {
	dir, _ := os.Getwd()
	if filepath.Base(dir) == "magefiles" {
		return filepath.Dir(dir)
	}
	return dir
}

func gitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func sanitizeBranch(branch string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-")
	return replacer.Replace(branch)
}

func ldflags(appName string) string {
	return fmt.Sprintf(
		"-X main.ApplicationName=%s -X transcoder/version.Version=%s -X transcoder/version.Commit=%s -X transcoder/version.Date=%s",
		appName, projectVersion, gitCommitSHA, buildDate,
	)
}

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = projectRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

// ---------------------------------------------------------------------------
// Dagger helpers
// ---------------------------------------------------------------------------

func daggerClient(ctx context.Context) (*dagger.Client, error) {
	return dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
}

func projectSource(client *dagger.Client) *dagger.Directory {
	return client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Exclude: []string{".git", "build", "dist", "magefiles"},
	})
}

func goContainer(client *dagger.Client, src *dagger.Directory) *dagger.Container {
	return client.Container().
		From("golang:1.25-bookworm").
		WithMountedCache("/root/.cache/go-build", client.CacheVolume("go-build")).
		WithMountedCache("/go/pkg/mod", client.CacheVolume("go-mod")).
		WithEnvVariable("CGO_ENABLED", "0").
		WithDirectory("/src", src).
		WithWorkdir("/src")
}

// lintContainer installs golangci-lint before mounting source for optimal layer caching.
func lintContainer(client *dagger.Client, src *dagger.Directory) *dagger.Container {
	return client.Container().
		From("golang:1.25-bookworm").
		WithMountedCache("/root/.cache/go-build", client.CacheVolume("go-build")).
		WithMountedCache("/go/pkg/mod", client.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/golangci-lint", client.CacheVolume("golangci-lint")).
		WithExec([]string{
			"sh", "-c",
			"curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/v2.1.6/install.sh | sh -s -- -b /usr/local/bin v2.1.6",
		}).
		WithDirectory("/src", src).
		WithWorkdir("/src")
}

// ---------------------------------------------------------------------------
// Build targets
// ---------------------------------------------------------------------------

// Build builds both server and worker Go binaries into dist/ via Dagger.
func Build(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	for _, component := range []string{"server", "worker"} {
		if err := buildBinaryWith(ctx, client, src, component); err != nil {
			return err
		}
	}
	return nil
}

// BuildServer builds the server binary into dist/transcoderd-server via Dagger.
func BuildServer(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildBinaryWith(ctx, client, projectSource(client), "server")
}

// BuildWorker builds the worker binary into dist/transcoderd-worker via Dagger.
func BuildWorker(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildBinaryWith(ctx, client, projectSource(client), "worker")
}

func buildBinaryWith(ctx context.Context, client *dagger.Client, src *dagger.Directory, component string) error {
	output := filepath.Join(projectRoot(), "dist", "transcoderd-"+component)
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}

	_, err := goContainer(client, src).
		WithEnvVariable("GOOS", runtime.GOOS).
		WithEnvVariable("GOARCH", runtime.GOARCH).
		WithExec([]string{
			"go", "build",
			"-ldflags", ldflags("transcoderd-" + component),
			"-o", "/output/transcoderd-" + component,
			"./" + component + "/main.go",
		}).
		File("/output/transcoderd-"+component).
		Export(ctx, output)
	if err != nil {
		return fmt.Errorf("build %s: %w", component, err)
	}
	fmt.Printf("Built %s\n", output)
	return nil
}

// ---------------------------------------------------------------------------
// Test targets
// ---------------------------------------------------------------------------

// Test runs unit tests with coverage via Dagger.
func Test(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-v",
		"-coverprofile=/tmp/coverage.out", "-covermode=atomic", "./..."}, false)
}

// TestRace runs unit tests with race detector via Dagger.
func TestRace(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-v", "-race",
		"-coverprofile=/tmp/coverage.out", "-covermode=atomic", "./..."}, false)
}

// TestShort runs unit tests in short mode via Dagger.
func TestShort(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-v", "-short", "./..."}, false)
}

// TestIntegration runs integration tests via Dagger (needs Docker socket for testcontainers).
func TestIntegration(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-tags=integration", "-v",
		"-timeout", "5m", "./integration/..."}, true)
}

// TestE2E runs the Docker e2e test via Dagger (requires built container images loaded in Docker).
func TestE2E(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	_, err = goContainer(client, src).
		WithUnixSocket("/var/run/docker.sock", client.Host().UnixSocket("/var/run/docker.sock")).
		WithEnvVariable("E2E_SERVER_IMAGE", fmt.Sprintf("%s:server-%s", imageName, gitBranchName)).
		WithEnvVariable("E2E_WORKER_IMAGE", fmt.Sprintf("%s:worker-%s", imageName, gitBranchName)).
		WithExec([]string{
			"go", "test", "-tags=integration", "-v",
			"-timeout", "30m", "-run", "TestDockerE2E", "./integration/...",
		}).
		Sync(ctx)
	return err
}

// TestAll runs unit + integration tests via Dagger.
func TestAll(ctx context.Context) error {
	if err := Test(ctx); err != nil {
		return err
	}
	return TestIntegration(ctx)
}

func runGoTest(ctx context.Context, args []string, needsDocker bool) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	ctr := goContainer(client, src)
	if needsDocker {
		ctr = ctr.WithUnixSocket("/var/run/docker.sock", client.Host().UnixSocket("/var/run/docker.sock"))
	}
	_, err = ctr.WithExec(args).Sync(ctx)
	return err
}

// ---------------------------------------------------------------------------
// Quality targets
// ---------------------------------------------------------------------------

// Lint runs golangci-lint via Dagger.
func Lint(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	_, err = lintContainer(client, src).
		WithExec([]string{"golangci-lint", "run"}).
		Sync(ctx)
	return err
}

// LintFix runs golangci-lint with auto-fix (local — modifies files in place).
func LintFix(ctx context.Context) error {
	return runCmd(ctx, "golangci-lint", "run", "--fix")
}

// Fmt formats Go code (local — modifies files in place).
func Fmt(ctx context.Context) error {
	return runCmd(ctx, "go", "fmt", "./...")
}

// ---------------------------------------------------------------------------
// Container targets (Dagger-powered)
// ---------------------------------------------------------------------------

// Container builds both server and worker Docker images via Dagger.
func Container(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	if err := buildContainerImage(ctx, client, src, "server"); err != nil {
		return err
	}
	return buildContainerImage(ctx, client, src, "worker")
}

// ContainerServer builds the server Docker image via Dagger.
func ContainerServer(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildContainerImage(ctx, client, projectSource(client), "server")
}

// ContainerWorker builds the worker Docker image via Dagger.
func ContainerWorker(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildContainerImage(ctx, client, projectSource(client), "worker")
}

// Publish builds and pushes both server and worker images via Dagger.
func Publish(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	for _, component := range []string{"server", "worker"} {
		ctr, err := containerImage(ctx, client, src, component)
		if err != nil {
			return err
		}
		for _, tag := range imageTags(component) {
			addr, err := ctr.Publish(ctx, tag)
			if err != nil {
				return fmt.Errorf("publish %s: %w", tag, err)
			}
			fmt.Printf("Published %s\n", addr)
		}
	}
	return nil
}

// PublishBuilderFfmpeg compiles FFmpeg from source and pushes the builder image (~50min).
func PublishBuilderFfmpeg(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	addr, err := builderFfmpegImage(client).Publish(ctx, imageName+":builder-ffmpeg")
	if err != nil {
		return fmt.Errorf("publish builder-ffmpeg: %w", err)
	}
	fmt.Printf("Published %s\n", addr)
	return nil
}

// PublishBuilderPgs builds the PgsToSrt .NET tool + tessdata and pushes the builder image (~5min).
func PublishBuilderPgs(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	addr, err := builderPgsImage(client).Publish(ctx, imageName+":builder-pgs")
	if err != nil {
		return fmt.Errorf("publish builder-pgs: %w", err)
	}
	fmt.Printf("Published %s\n", addr)
	return nil
}

// ---------------------------------------------------------------------------
// CI target
// ---------------------------------------------------------------------------

// CI runs the full CI pipeline: lint → test → build → container → e2e.
func CI(ctx context.Context) error {
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"lint", Lint},
		{"test", Test},
		{"build", Build},
		{"container", Container},
		{"e2e", TestE2E},
	}
	for _, s := range steps {
		fmt.Printf("\n=== CI: %s ===\n", s.name)
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("ci step %q failed: %w", s.name, err)
		}
	}
	fmt.Println("\n✅ CI pipeline passed!")
	return nil
}

// ---------------------------------------------------------------------------
// Dagger container build internals
// ---------------------------------------------------------------------------

func buildContainerImage(ctx context.Context, client *dagger.Client, src *dagger.Directory, component string) error {
	ctr, err := containerImage(ctx, client, src, component)
	if err != nil {
		return err
	}

	for _, tag := range imageTags(component) {
		tarPath := filepath.Join(projectRoot(), "dist", component+"-image.tar")
		_, err := ctr.AsTarball(dagger.ContainerAsTarballOpts{
			ForcedCompression: dagger.ImageLayerCompressionGzip,
		}).Export(ctx, tarPath)
		if err != nil {
			return fmt.Errorf("export %s: %w", component, err)
		}
		fmt.Printf("Exported %s → %s\n", tag, tarPath)

		if err := runCmd(ctx, "docker", "load", "-i", tarPath); err != nil {
			fmt.Printf("Warning: docker load failed (Docker may not be available): %v\n", err)
		} else {
			if err := runCmd(ctx, "docker", "tag", component+":latest", tag); err != nil {
				fmt.Printf("Warning: docker tag failed: %v\n", err)
			}
		}
	}
	return nil
}

func containerImage(ctx context.Context, client *dagger.Client, src *dagger.Directory, component string) (*dagger.Container, error) {
	goBinary := goContainer(client, src).
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{
			"go", "build",
			"-ldflags", ldflags("transcoderd-" + component),
			"-o", "/output/transcoderd-" + component,
			"./" + component + "/main.go",
		}).
		File("/output/transcoderd-" + component)

	ffmpegBins := client.Container().
		From(imageName + ":builder-ffmpeg")

	base := client.Container().
		From("debian:trixie-20241223-slim").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{
			"apt-get", "install", "-y",
			"ca-certificates", "mkvtoolnix", "libva-drm2", "wget",
		}).
		WithExec([]string{"rm", "-rf", "/var/lib/apt/lists/*"}).
		WithFile("/usr/bin/ffmpeg", ffmpegBins.File("/app/ffmpeg-build-script/workspace/bin/ffmpeg")).
		WithFile("/usr/bin/ffprobe", ffmpegBins.File("/app/ffmpeg-build-script/workspace/bin/ffprobe")).
		WithFile("/usr/bin/ffplay", ffmpegBins.File("/app/ffmpeg-build-script/workspace/bin/ffplay"))

	switch component {
	case "server":
		return base.
			WithFile("/app/transcoderd-server", goBinary).
			WithEntrypoint([]string{"/app/transcoderd-server"}), nil

	case "worker":
		pgsBins := client.Container().
			From(imageName + ":builder-pgs")

		return base.
			WithDirectory("/app/tessdata", pgsBins.Directory("/src/tessdata")).
			WithDirectory("/app", pgsBins.Directory("/src/PgsToSrt/out")).
			WithExec([]string{"apt-get", "update"}).
			WithExec([]string{
				"apt-get", "install", "-y", "libtesseract-dev",
			}).
			WithExec([]string{"apt-get", "clean"}).
			WithExec([]string{"rm", "-rf", "/var/lib/apt/lists/*"}).
			WithExec([]string{
				"bash", "-c",
				"wget https://dot.net/v1/dotnet-install.sh -O /tmp/dotnet-install.sh && " +
					"chmod +x /tmp/dotnet-install.sh && " +
					"/tmp/dotnet-install.sh --channel 8.0 --runtime dotnet --install-dir /usr/share/dotnet && " +
					"ln -sf /usr/share/dotnet/dotnet /usr/bin/dotnet && " +
					"rm /tmp/dotnet-install.sh",
			}).
			WithFile("/app/transcoderd-worker", goBinary).
			WithEntrypoint([]string{"/app/transcoderd-worker"}), nil

	default:
		return nil, fmt.Errorf("unknown component: %s", component)
	}
}

// builderFfmpegImage replicates the Dockerfile builder-ffmpeg stage as a pure Dagger pipeline.
// Base: ubuntu:noble-20251013 with FFmpeg compiled from source (~50min).
func builderFfmpegImage(client *dagger.Client) *dagger.Container {
	return client.Container().
		From("ubuntu:noble-20251013").
		WithEnvVariable("DEBIAN_FRONTEND", "noninteractive").
		WithExec([]string{
			"bash", "-c",
			"apt-get update && " +
				"apt-get -y --no-install-recommends install git build-essential curl ca-certificates libva-dev " +
				"python3 python-is-python3 ninja-build meson && " +
				"apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/doc/* && " +
				"update-ca-certificates",
		}).
		WithWorkdir("/app").
		WithExec([]string{
			"git", "clone", "--depth", "1", "--branch", "v1.59",
			"https://github.com/markus-perl/ffmpeg-build-script.git",
		}).
		WithWorkdir("/app/ffmpeg-build-script").
		WithExec([]string{
			"bash", "-c",
			"SKIPINSTALL=yes ./build-ffmpeg --build --enable-gpl-and-non-free",
		}).
		WithExec([]string{"rm", "-rf", "packages"})
}

// builderPgsImage replicates the Dockerfile builder-pgs stage as a pure Dagger pipeline.
// Base: dotnet/sdk:8.0.419 with PgsToSrt + tessdata (~5min).
func builderPgsImage(client *dagger.Client) *dagger.Container {
	const (
		tessdataVersion = "ced78752cc61322fb554c280d13360b35b8684e4"
		pgstosrtVersion = "ef11919491b5c98f9dcdaf13d721596e60efb7ed"
	)

	return client.Container().
		From("mcr.microsoft.com/dotnet/sdk:8.0.419").
		WithWorkdir("/src").
		WithExec([]string{
			"bash", "-c",
			"apt-get -y update && apt-get -y upgrade && " +
				"apt-get -y install automake ca-certificates g++ libtool libtesseract-dev " +
				"make pkg-config wget unzip libc6-dev && " +
				"apt-get clean && rm -rf /var/lib/apt/lists/*",
		}).
		WithExec([]string{
			"bash", "-c",
			fmt.Sprintf(
				"wget -O tessdata.zip 'https://github.com/tesseract-ocr/tessdata/archive/%s.zip' && "+
					"unzip tessdata.zip && rm tessdata.zip && "+
					"mv tessdata-%s tessdata",
				tessdataVersion, tessdataVersion,
			),
		}).
		WithExec([]string{
			"bash", "-c",
			fmt.Sprintf(
				"wget -O pgstosrt.zip 'https://github.com/Tentacule/PgsToSrt/archive/%s.zip' && "+
					"unzip pgstosrt.zip && rm pgstosrt.zip && "+
					"cd PgsToSrt-%s/src && "+
					"dotnet restore && "+
					"dotnet publish -c Release -f net8.0 -o /src/PgsToSrt/out",
				pgstosrtVersion, pgstosrtVersion,
			),
		})
}

func imageTags(component string) []string {
	return []string{
		fmt.Sprintf("%s:%s-%s", imageName, component, projectVersion),
		fmt.Sprintf("%s:%s-%s", imageName, component, gitBranchName),
	}
}
