// Package main provides the Mage build targets for TranscoderD.
// All build, test, and lint operations run inside Dagger containers for
// reproducibility and content-addressed caching. Mage is the CLI orchestrator.
//
// Usage:
//
//	mage build:all       # Build server + worker Go binaries
//	mage test:unit       # Run unit tests with coverage
//	mage test:e2e        # Run end-to-end tests
//	mage lint:check      # Run golangci-lint
//	mage docker:all      # Build Docker images via Dagger
//	mage publish:all     # Build and push all Docker images
//	mage -l              # List all targets
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
	"github.com/magefile/mage/mg"
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

const e2eServerConfig = `server:
  database:
    host: postgres
    port: 5432
    user: test
    password: test
    scheme: transcoderd_e2e
    driver: postgres
  scheduler:
    sourcePath: /source
    deleteOnComplete: false
    minFileSize: 100
    scheduleTime: 5s
    jobTimeout: 10m
web:
  token: e2e-test-token
  port: 8080
`

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
	opts := []dagger.ClientOpt{}
	if os.Getenv("DAGGER_LOG") == "1" {
		opts = append(opts, dagger.WithLogOutput(os.Stderr))
	}
	return dagger.Connect(ctx, opts...)
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

func lintContainer(client *dagger.Client, src *dagger.Directory) *dagger.Container {
	return client.Container().
		From("golang:1.25-bookworm").
		WithMountedCache("/root/.cache/go-build", client.CacheVolume("go-build")).
		WithMountedCache("/go/pkg/mod", client.CacheVolume("go-mod")).
		WithMountedCache("/root/.cache/golangci-lint", client.CacheVolume("golangci-lint")).
		WithExec([]string{
			"sh", "-c",
			"curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/v2.11.4/install.sh | sh -s -- -b /usr/local/bin v2.11.4",
		}).
		WithDirectory("/src", src).
		WithWorkdir("/src")
}

func runGoTest(ctx context.Context, args []string) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	_, err = goContainer(client, src).WithExec(args).Sync(ctx)
	return err
}

// ---------------------------------------------------------------------------
// Build namespace
// ---------------------------------------------------------------------------

// Build contains targets for compiling Go binaries.
type Build mg.Namespace

// All builds both server and worker Go binaries into dist/ via Dagger.
func (Build) All(ctx context.Context) error {
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

// Server builds the server binary into dist/transcoderd-server via Dagger.
func (Build) Server(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildBinaryWith(ctx, client, projectSource(client), "server")
}

// Worker builds the worker binary into dist/transcoderd-worker via Dagger.
func (Build) Worker(ctx context.Context) error {
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
// Test namespace
// ---------------------------------------------------------------------------

// Test contains targets for running tests.
type Test mg.Namespace

// Unit runs unit tests with coverage via Dagger.
func (Test) Unit(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-v",
		"-coverprofile=/tmp/coverage.out", "-covermode=atomic", "./..."})
}

// Race runs unit tests with race detector via Dagger.
func (Test) Race(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-v", "-race",
		"-coverprofile=/tmp/coverage.out", "-covermode=atomic", "./..."})
}

// Short runs unit tests in short mode via Dagger.
func (Test) Short(ctx context.Context) error {
	return runGoTest(ctx, []string{"go", "test", "-v", "-short", "./..."})
}

// Integration runs integration tests via Dagger with Postgres as a Dagger Service.
func (Test) Integration(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)

	postgresSvc := client.Container().
		From("postgres:16-alpine").
		WithEnvVariable("POSTGRES_DB", "transcoderd_test").
		WithEnvVariable("POSTGRES_USER", "test").
		WithEnvVariable("POSTGRES_PASSWORD", "test").
		WithExposedPort(5432).
		AsService()

	_, err = goContainer(client, src).
		WithServiceBinding("postgres", postgresSvc).
		WithEnvVariable("INTEGRATION_PG_HOST", "postgres").
		WithEnvVariable("INTEGRATION_PG_PORT", "5432").
		WithEnvVariable("INTEGRATION_PG_USER", "test").
		WithEnvVariable("INTEGRATION_PG_PASSWORD", "test").
		WithEnvVariable("INTEGRATION_PG_DATABASE", "transcoderd_test").
		WithExec([]string{
			"go", "test", "-tags=integration", "-v", "-timeout", "5m",
			"-run", "TestServerWorkerIntegration", "./integration/...",
		}).
		Sync(ctx)
	return err
}

// E2e runs the full end-to-end test using Dagger Services.
// Postgres, server, and worker run as Dagger services; the test binary
// communicates with the server over HTTP only.
func (Test) E2e(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	src := projectSource(client)
	fixtureFile := client.Host().File(
		filepath.Join(projectRoot(), "integration", "testdata", "e2e_fixture.mkv"),
	)

	// 1. Postgres service
	postgresSvc := client.Container().
		From("postgres:16-alpine").
		WithEnvVariable("POSTGRES_DB", "transcoderd_e2e").
		WithEnvVariable("POSTGRES_USER", "test").
		WithEnvVariable("POSTGRES_PASSWORD", "test").
		WithExposedPort(5432).
		AsService()

	// 2. Server service
	serverCtr, err := containerImage(ctx, client, src, "server")
	if err != nil {
		return fmt.Errorf("build server image: %w", err)
	}
	serverSvc := serverCtr.
		WithServiceBinding("postgres", postgresSvc).
		WithNewFile("/etc/transcoderd/config.yml", e2eServerConfig).
		WithFile("/source/e2e_fixture.mkv", fixtureFile).
		WithExposedPort(8080).
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{"/app/transcoderd-server", "--noUpdates"}).
		AsService()

	// 3. Worker binary (background process in test runner)
	goBinary := goContainer(client, src).
		WithEnvVariable("GOOS", "linux").
		WithEnvVariable("GOARCH", "amd64").
		WithEnvVariable("CACHE_BUSTER", buildDate).
		WithExec([]string{
			"go", "build",
			"-ldflags", ldflags("transcoderd-worker"),
			"-o", "/output/transcoderd-worker",
			"./worker/main.go",
		}).
		File("/output/transcoderd-worker")

	ffmpegBins := builderFfmpegImage(client)
	pgsBins := builderPgsImage(client)

	// 4. Test runner
	out, err := goContainer(client, src).
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "ca-certificates", "mkvtoolnix", "libtesseract-dev", "wget", "curl"}).
		WithExec([]string{"update-ca-certificates"}).
		WithExec([]string{"sh", "-c", "curl -ksSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh && chmod +x /tmp/dotnet-install.sh && /tmp/dotnet-install.sh --channel 8.0 --runtime dotnet --install-dir /usr/share/dotnet && ln -sf /usr/share/dotnet/dotnet /usr/bin/dotnet && rm /tmp/dotnet-install.sh"}).
		WithFile("/usr/bin/ffmpeg", ffmpegBins.File("/app/ffmpeg-build-script/workspace/bin/ffmpeg")).
		WithFile("/usr/bin/ffprobe", ffmpegBins.File("/app/ffmpeg-build-script/workspace/bin/ffprobe")).
		WithDirectory("/app/tessdata", pgsBins.Directory("/src/tessdata")).
		WithDirectory("/app", pgsBins.Directory("/src/PgsToSrt/out")).
		WithFile("/app/transcoderd-worker", goBinary).
		WithServiceBinding("server", serverSvc).
		WithEnvVariable("E2E_SERVER_URL", "http://server:8080").
		WithEnvVariable("E2E_TOKEN", "e2e-test-token").
		WithEnvVariable("E2E_SOURCE_PATH", "e2e_fixture.mkv").
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{
			"sh", "-c",
			"/app/transcoderd-worker" +
				" --noUpdates" +
				" --web.token e2e-test-token" +
				" --web.domain http://server:8080" +
				" --worker.name e2e-worker" +
				" --worker.ffmpegConfig.videoPreset ultrafast" +
				" --worker.ffmpegConfig.videoCRF 35" +
				" --worker.ffmpegConfig.videoCodec libx265" +
				" --worker.ffmpegConfig.audioCodec aac" +
				" --worker.ffmpegConfig.videoProfile main10" +
				" --worker.verifyDeltaTime 5" +
				" --worker.threads 2" +
				" > /tmp/worker.log 2>&1 &" +
				" sleep 3 && echo '=== WORKER STARTUP ===' && cat /tmp/worker.log && echo '=== END WORKER STARTUP ===' &&" +
				" go test -tags=integration -v -timeout 15m -run TestE2E ./integration/... 2>&1; EXIT=$?;" +
				" echo '=== WORKER LOG ==='; cat /tmp/worker.log; exit $EXIT",
		}).
		Stdout(ctx)
	fmt.Println(out)
	return err
}

// All runs unit + integration tests via Dagger.
func (Test) All(ctx context.Context) error {
	t := Test{}
	if err := t.Unit(ctx); err != nil {
		return err
	}
	return t.Integration(ctx)
}

// ---------------------------------------------------------------------------
// Lint namespace
// ---------------------------------------------------------------------------

// Lint contains targets for code quality checks.
type Lint mg.Namespace

// Check runs golangci-lint via Dagger.
func (Lint) Check(ctx context.Context) error {
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

// Fix runs golangci-lint with auto-fix (local — modifies files in place).
func (Lint) Fix(ctx context.Context) error {
	return runCmd(ctx, "golangci-lint", "run", "--fix")
}

// Fmt formats Go code (local — modifies files in place).
func (Lint) Fmt(ctx context.Context) error {
	return runCmd(ctx, "go", "fmt", "./...")
}

// ---------------------------------------------------------------------------
// Docker namespace
// ---------------------------------------------------------------------------

// Docker contains targets for building container images locally.
type Docker mg.Namespace

// All builds both server and worker Docker images via Dagger.
func (Docker) All(ctx context.Context) error {
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

// Server builds the server Docker image via Dagger.
func (Docker) Server(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildContainerImage(ctx, client, projectSource(client), "server")
}

// Worker builds the worker Docker image via Dagger.
func (Docker) Worker(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()
	return buildContainerImage(ctx, client, projectSource(client), "worker")
}

// Ffmpeg builds the FFmpeg builder image locally (~50min).
func (Docker) Ffmpeg(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	tarPath := filepath.Join(projectRoot(), "dist", "builder-ffmpeg.tar")
	if err := os.MkdirAll(filepath.Dir(tarPath), 0o755); err != nil {
		return err
	}
	_, err = builderFfmpegImage(client).
		AsTarball(dagger.ContainerAsTarballOpts{
			ForcedCompression: dagger.ImageLayerCompressionGzip,
		}).
		Export(ctx, tarPath)
	if err != nil {
		return fmt.Errorf("export builder-ffmpeg: %w", err)
	}
	fmt.Printf("Exported builder-ffmpeg → %s\n", tarPath)
	return nil
}

// Pgs builds the PGS builder image locally (~5min).
func (Docker) Pgs(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return fmt.Errorf("dagger connect: %w", err)
	}
	defer client.Close()

	tarPath := filepath.Join(projectRoot(), "dist", "builder-pgs.tar")
	if err := os.MkdirAll(filepath.Dir(tarPath), 0o755); err != nil {
		return err
	}
	_, err = builderPgsImage(client).
		AsTarball(dagger.ContainerAsTarballOpts{
			ForcedCompression: dagger.ImageLayerCompressionGzip,
		}).
		Export(ctx, tarPath)
	if err != nil {
		return fmt.Errorf("export builder-pgs: %w", err)
	}
	fmt.Printf("Exported builder-pgs → %s\n", tarPath)
	return nil
}

// ---------------------------------------------------------------------------
// Publish namespace
// ---------------------------------------------------------------------------

// Publish contains targets for pushing container images to the registry.
type Publish mg.Namespace

// All pushes all images (server, worker, ffmpeg builder, pgs builder).
func (Publish) All(ctx context.Context) error {
	p := Publish{}
	if err := p.App(ctx); err != nil {
		return err
	}
	if err := p.Ffmpeg(ctx); err != nil {
		return err
	}
	return p.Pgs(ctx)
}

// App pushes both server and worker images via Dagger.
func (Publish) App(ctx context.Context) error {
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

// Ffmpeg compiles FFmpeg from source and pushes the builder image (~50min).
func (Publish) Ffmpeg(ctx context.Context) error {
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

// Pgs builds PgsToSrt + tessdata and pushes the builder image (~5min).
func (Publish) Pgs(ctx context.Context) error {
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
		WithEnvVariable("CACHE_BUSTER", buildDate).
		WithExec([]string{
			"go", "build",
			"-ldflags", ldflags("transcoderd-" + component),
			"-o", "/output/transcoderd-" + component,
			"./" + component + "/main.go",
		}).
		File("/output/transcoderd-" + component)

	ffmpegBins := builderFfmpegImage(client)

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
		pgsBins := builderPgsImage(client)

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
