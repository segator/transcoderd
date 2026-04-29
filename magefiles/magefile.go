// Package main provides the Mage build targets for TranscoderD.
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
	// magefiles/ lives one level below the project root.
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

func daggerClient(ctx context.Context) (*dagger.Client, error) {
	return dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
}

// ---------------------------------------------------------------------------
// Build targets
// ---------------------------------------------------------------------------

// Build builds both server and worker Go binaries into dist/.
func Build(ctx context.Context) error {
	fmt.Println("Building server and worker binaries...")
	if err := BuildServer(ctx); err != nil {
		return err
	}
	return BuildWorker(ctx)
}

// BuildServer builds the server binary into dist/transcoderd-server.
func BuildServer(ctx context.Context) error {
	return buildBinary(ctx, "server")
}

// BuildWorker builds the worker binary into dist/transcoderd-worker.
func BuildWorker(ctx context.Context) error {
	return buildBinary(ctx, "worker")
}

func buildBinary(ctx context.Context, component string) error {
	output := filepath.Join("dist", "transcoderd-"+component)
	fmt.Printf("Building %s\n", output)

	if err := os.MkdirAll(filepath.Join(projectRoot(), "dist"), 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "go", "build",
		"-ldflags", ldflags("transcoderd-"+component),
		"-o", output,
		"./"+component+"/main.go",
	)
	cmd.Dir = projectRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	return cmd.Run()
}

// ---------------------------------------------------------------------------
// Test targets
// ---------------------------------------------------------------------------

// Test runs unit tests with coverage.
func Test(ctx context.Context) error {
	fmt.Println("Running unit tests...")
	if err := os.MkdirAll(filepath.Join(projectRoot(), "build"), 0o755); err != nil {
		return err
	}
	return runCmd(ctx, "go", "test", "-v",
		"-coverprofile=build/coverage.out", "-covermode=atomic", "./...")
}

// TestRace runs unit tests with race detector.
func TestRace(ctx context.Context) error {
	fmt.Println("Running unit tests with race detector...")
	if err := os.MkdirAll(filepath.Join(projectRoot(), "build"), 0o755); err != nil {
		return err
	}
	return runCmd(ctx, "go", "test", "-v", "-race",
		"-coverprofile=build/coverage.out", "-covermode=atomic", "./...")
}

// TestShort runs unit tests in short mode.
func TestShort(ctx context.Context) error {
	fmt.Println("Running unit tests (short)...")
	return runCmd(ctx, "go", "test", "-v", "-short", "./...")
}

// TestIntegration runs integration tests (requires Docker for testcontainers).
func TestIntegration(ctx context.Context) error {
	fmt.Println("Running integration tests...")
	return runCmd(ctx, "go", "test", "-tags=integration", "-v", "-timeout", "5m", "./integration/...")
}

// TestE2E runs the Docker e2e test (requires built container images).
func TestE2E(ctx context.Context) error {
	fmt.Println("Running e2e tests...")
	cmd := exec.CommandContext(ctx, "go", "test",
		"-tags=integration", "-v", "-timeout", "30m",
		"-run", "TestDockerE2E", "./integration/...",
	)
	cmd.Dir = projectRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("E2E_SERVER_IMAGE=%s:server-%s", imageName, gitBranchName),
		fmt.Sprintf("E2E_WORKER_IMAGE=%s:worker-%s", imageName, gitBranchName),
	)
	return cmd.Run()
}

// TestAll runs unit + integration tests.
func TestAll(ctx context.Context) error {
	if err := Test(ctx); err != nil {
		return err
	}
	return TestIntegration(ctx)
}

// ---------------------------------------------------------------------------
// Quality targets
// ---------------------------------------------------------------------------

// Lint runs golangci-lint.
func Lint(ctx context.Context) error {
	fmt.Println("Running linter...")
	return runCmd(ctx, "golangci-lint", "run")
}

// LintFix runs golangci-lint with auto-fix.
func LintFix(ctx context.Context) error {
	fmt.Println("Running linter with auto-fix...")
	return runCmd(ctx, "golangci-lint", "run", "--fix")
}

// Fmt formats Go code.
func Fmt(ctx context.Context) error {
	fmt.Println("Formatting Go code...")
	return runCmd(ctx, "go", "fmt", "./...")
}

// ---------------------------------------------------------------------------
// Container targets (Dagger-powered)
// ---------------------------------------------------------------------------

// Container builds both server and worker Docker images via Dagger.
func Container(ctx context.Context) error {
	fmt.Println("Building container images via Dagger...")
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Exclude: []string{".git", "build", "magefiles"},
	})

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

	src := client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Exclude: []string{".git", "build", "magefiles"},
	})
	return buildContainerImage(ctx, client, src, "server")
}

// ContainerWorker builds the worker Docker image via Dagger.
func ContainerWorker(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Exclude: []string{".git", "build", "magefiles"},
	})
	return buildContainerImage(ctx, client, src, "worker")
}

// Publish builds and pushes both server and worker images via Dagger.
func Publish(ctx context.Context) error {
	fmt.Println("Publishing container images via Dagger...")
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Exclude: []string{".git", "build", "magefiles"},
	})

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
	fmt.Println("Publishing FFmpeg builder image via Dagger (~50min)...")
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	ctr := builderFfmpegImage(client)

	addr, err := ctr.Publish(ctx, imageName+":builder-ffmpeg")
	if err != nil {
		return fmt.Errorf("publish builder-ffmpeg: %w", err)
	}
	fmt.Printf("Published %s\n", addr)
	return nil
}

// PublishBuilderPgs builds the PgsToSrt .NET tool + tessdata and pushes the builder image (~5min).
func PublishBuilderPgs(ctx context.Context) error {
	fmt.Println("Publishing PGS builder image via Dagger (~5min)...")
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	ctr := builderPgsImage(client)

	addr, err := ctr.Publish(ctx, imageName+":builder-pgs")
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
	fmt.Println("Running full CI pipeline...")
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

		// Load into local Docker daemon
		if err := runCmd(ctx, "docker", "load", "-i", tarPath); err != nil {
			fmt.Printf("Warning: docker load failed (Docker may not be available): %v\n", err)
		} else {
			// Tag the loaded image
			if err := runCmd(ctx, "docker", "tag", component+":latest", tag); err != nil {
				fmt.Printf("Warning: docker tag failed: %v\n", err)
			}
		}
	}
	return nil
}

func containerImage(ctx context.Context, client *dagger.Client, src *dagger.Directory, component string) (*dagger.Container, error) {
	// Build the Go binary inside Dagger for reproducibility
	goBinary := client.Container().
		From("golang:1.25-bookworm").
		WithDirectory("/src", src).
		WithWorkdir("/src").
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").
		WithMountedCache("/root/.cache/go-build", client.CacheVolume("go-build")).
		WithMountedCache("/go/pkg/mod", client.CacheVolume("go-mod")).
		WithExec([]string{
			"go", "build",
			"-ldflags", ldflags("transcoderd-" + component),
			"-o", "/output/transcoderd-" + component,
			"./" + component + "/main.go",
		}).
		File("/output/transcoderd-" + component)

	// Pre-built FFmpeg image (pulled from registry — never rebuilt in normal CI)
	ffmpegBins := client.Container().
		From(imageName + ":builder-ffmpeg")

	// Base runtime: debian + FFmpeg binaries
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
		// Pre-built PGS image (pulled from registry)
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
