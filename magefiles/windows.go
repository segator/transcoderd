package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dagger.io/dagger"
	"golang.org/x/sync/errgroup"
)

// Windows builds the native Windows x64 worker and its portable tool bundle.
func (Build) Windows(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	source := projectSource(client)
	tools, err := windowsWorkerTools(ctx, client)
	if err != nil {
		return err
	}
	binary := windowsWorkerBinary(client, source)
	if _, err := binary.Export(ctx, filepath.Join(projectRoot(), "dist", "transcoderd-worker-windows-amd64.exe")); err != nil {
		return err
	}
	archive, name := windowsPortableArchive(tools, binary)
	_, err = archive.Export(ctx, filepath.Join(projectRoot(), "dist", name))
	return err
}

// Compress the pinned files once. Adding the current worker rewrites the ZIP
// directory and compresses one entry, even when the worker changes on every PR.
func windowsPortableArchive(tools *dagger.Container, binary *dagger.File) (*dagger.File, string) {
	archive := "transcoderd-worker-" + projectVersion + "-windows-amd64.zip"
	base := tools.WithWorkdir("/bundle").
		WithExec([]string{"zip", "-q", "-3", "-r", "/portable.zip", "."})
	return base.
		WithFile("/bundle/transcoderd-worker.exe", binary).
		WithExec([]string{"mv", "/portable.zip", "/" + archive}).
		WithExec([]string{"zip", "-q", "-3", "/" + archive, "transcoderd-worker.exe"}).
		File("/" + archive), archive
}

// WindowsBinary cross-compiles only the worker executable, without media tools.
func (Build) WindowsBinary(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = windowsWorkerBinary(client, projectSource(client)).Export(ctx, filepath.Join(projectRoot(), "dist", "transcoderd-worker-windows-amd64.exe"))
	return err
}

func windowsWorkerBinary(client *dagger.Client, src *dagger.Directory) *dagger.File {
	return windowsGoBinary(client, src, "./worker", "transcoderd-worker", projectVersion, "", "windows")
}

// WindowsBinaries exports native Windows unit-test executables. Run them with
// scripts/windows/run-tests.ps1 on Windows; cross-compilation is not execution.
func (Test) WindowsBinaries(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = windowsTestBinaries(client, projectSource(client)).Export(ctx, filepath.Join(projectRoot(), "dist", "windows-tests"))
	return err
}

func windowsE2eBinaries(client *dagger.Client, source *dagger.Directory) *dagger.Container {
	return goContainer(client, source).
		WithEnvVariable("GOOS", "windows").WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{"go", "test", "-c", "-tags=integration", "-o", "/output/windows-bundle.test.exe", "./integration"}).
		WithExec([]string{"go", "build", "-ldflags", ldflags("transcoderd-server"), "-o", "/output/windows-e2e-server.exe", "./server"})
}

func windowsTestBinaries(client *dagger.Client, source *dagger.Directory) *dagger.Directory {
	return windowsE2eBinaries(client, source).
		WithExec([]string{"bash", "-c", `set -eu; for pkg in $(go list ./cmd/... ./helper/... ./model/... ./update/... ./version/... ./worker/... ./frontend/...); do name=$(echo "$pkg" | tr / _); go test -c -o "/output/$name.test.exe" "$pkg"; done`}).
		Directory("/output")
}

func windowsCompilePlan(value string) (budget, parallel, jobs int, err error) {
	if value == "" {
		value = "16"
	}
	budget, err = strconv.Atoi(value)
	if err != nil || budget < 1 {
		return 0, 0, 0, fmt.Errorf("WINDOWS_BUILD_JOBS must be a positive integer, got %q", value)
	}
	parallel = min(4, budget)
	return budget, parallel, budget / parallel, nil
}

func windowsFFmpeg(ctx context.Context, client *dagger.Client) (*dagger.Container, error) {
	budget, parallel, jobs, err := windowsCompilePlan(os.Getenv("WINDOWS_BUILD_JOBS"))
	if err != nil {
		return nil, err
	}
	base := client.Container().From("debian:trixie-slim").
		WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates curl xz-utils make nasm cmake ninja-build pkg-config git gcc-mingw-w64-x86-64-posix g++-mingw-w64-x86-64-posix && rm -rf /var/lib/apt/lists/*"}).
		WithFile("/build-ffmpeg.sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "windows", "build-ffmpeg.sh"))).
		WithFile("/fetch.sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "windows", "fetch.sh")))
	stages := make([]*dagger.Container, 0, 5)
	builds, buildCtx := errgroup.WithContext(ctx)
	builds.SetLimit(parallel)
	for _, name := range []string{"aom", "svt", "x265", "fdk", "nvcodec"} {
		// Each stage has only its own sources as inputs. Changing SVT cannot
		// invalidate the cached AOM, x265, AAC or NVCodec installations.
		stage := base.WithEnvVariable("BUILD_JOBS", strconv.Itoa(jobs)).
			WithFile("/"+name+".sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "windows", name+".sh"))).
			WithExec([]string{"bash", "/build-ffmpeg.sh", name})
		stages = append(stages, stage)
		builds.Go(func() error {
			if _, err := stage.Sync(buildCtx); err != nil {
				return fmt.Errorf("build Windows %s: %w", name, err)
			}
			return nil
		})
	}
	if err := builds.Wait(); err != nil {
		return nil, err
	}
	// FFmpeg executes native table generators and links the independently
	// compiled libraries. Only this final compilation needs all dependencies.
	container := base.WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*"})
	for _, stage := range stages {
		container = container.WithDirectory("/opt/windows", stage.Directory("/opt/windows")).
			WithDirectory("/output", stage.Directory("/output"))
	}
	return container.WithEnvVariable("BUILD_JOBS", strconv.Itoa(budget)).
		WithFile("/ffmpeg.sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "windows", "ffmpeg.sh"))).
		WithExec([]string{"bash", "/build-ffmpeg.sh", "ffmpeg"}), nil
}

func windowsPGS(client *dagger.Client) *dagger.Container {
	return pgsBuilderImage(client).WithExec([]string{"bash", "-c", fmt.Sprintf("cd /src/PgsToSrt-%s/src && dotnet publish PgsToSrt/PgsToSrt.csproj -c Release -f net8.0 -r win-x64 --self-contained true -o /windows-pgs", pgstosrtVersion)})
}

func windowsWorkerTools(ctx context.Context, client *dagger.Client) (*dagger.Container, error) {
	ffmpeg, err := windowsFFmpeg(ctx, client)
	if err != nil {
		return nil, err
	}
	pgs := windowsPGS(client)
	return client.Container().From("debian:trixie-slim").
		WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates curl 7zip zip && rm -rf /var/lib/apt/lists/*"}).
		WithExec([]string{"bash", "-c", fmt.Sprintf("curl --fail --location --retry 5 https://mkvtoolnix.download/windows/releases/103.0/mkvtoolnix-64-bit-103.0.7z -o /mkv.7z && echo '%s  /mkv.7z' | sha256sum --check --status && 7z x /mkv.7z -o/mkv && rm /mkv.7z", windowsMKVSHA256)}).
		WithDirectory("/bundle", ffmpeg.Directory("/output")).
		WithDirectory("/bundle/pgs", pgs.Directory("/windows-pgs")).
		WithDirectory("/bundle/tessdata", pgs.Directory("/src/tessdata")).
		// Linux Tesseract's config/font symlinks are not used by PgsToSrt OCR.
		// Ship regular trained-data files; Windows cannot resolve those links.
		WithExec([]string{"bash", "-c", "find /bundle/tessdata -type l -delete"}).
		WithFile("/bundle/licenses/TranscoderD.txt", client.Host().File(filepath.Join(projectRoot(), "LICENSE"))).
		WithNewFile("/bundle/licenses/PgsToSrt-source.txt", "PgsToSrt source: https://github.com/Tentacule/PgsToSrt/tree/"+pgstosrtVersion+"\nLocal patch: pgs-patches/pgstosrt-bitmap-bounds.patch\n").
		WithExec([]string{"bash", "-c", "cp -a /mkv/mkvtoolnix/mkvextract.exe /bundle/bin/ && find /mkv/mkvtoolnix -maxdepth 1 -iname '*.dll' -exec cp -t /bundle/bin/ {} + && cp -a /mkv/mkvtoolnix/doc/COPYING.txt /bundle/licenses/mkvtoolnix.txt"}).
		WithFile("/bundle/README.md", client.Host().File(filepath.Join(projectRoot(), "docs", "worker-windows.md"))).
		WithFile("/bundle/windows-worker-validation.md", client.Host().File(filepath.Join(projectRoot(), "docs", "windows-worker-validation.md"))), nil
}

const windowsMKVSHA256 = "01d76dbfdbd5615d9e02f69b0710d1429a902d47a98c8eee33bd884c4fcb3cbf"

// Windows checks Windows-specific service, updater and worker code via Dagger.
func (Lint) Windows(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = lintContainer(client, projectSource(client)).WithEnvVariable("GOOS", "windows").WithEnvVariable("GOARCH", "amd64").WithExec([]string{"golangci-lint", "run"}).Sync(ctx)
	return err
}

// WindowsE2e runs a native Windows worker against an isolated Linux server and
// PostgreSQL. Invoke from WSL after build:windows. WINDOWS_BUNDLE is the local
// Windows directory containing an extracted bundle; WINDOWS_CODEC defaults to
// libsvtav1 and can select av1_nvenc on a compatible NVIDIA machine.
func (Test) WindowsE2e(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	bundle := os.Getenv("WINDOWS_BUNDLE")
	if bundle == "" {
		return fmt.Errorf("WINDOWS_BUNDLE must point to an extracted native Windows worker bundle")
	}
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	src := projectSource(client)
	postgres := client.Container().From("postgres:16-alpine").
		WithEnvVariable("POSTGRES_DB", "transcoderd_e2e").WithEnvVariable("POSTGRES_USER", "test").
		WithEnvVariable("POSTGRES_PASSWORD", "test").WithExposedPort(5432).AsService()
	serverBinary := goContainer(client, src).WithEnvVariable("GOOS", "linux").WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{"go", "build", "-ldflags", ldflags("transcoderd-server"), "-o", "/server", "./server"}).File("/server")
	server := client.Container().From("debian:trixie-slim").
		WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg && rm -rf /var/lib/apt/lists/*"}).
		WithFile("/app/server", serverBinary).WithNewFile("/etc/transcoderd/config.yml", e2eServerConfig).
		WithFile("/source/e2e_fixture.mkv", client.Host().File(filepath.Join(projectRoot(), "integration", "testdata", "e2e_fixture.mkv"))).
		WithServiceBinding("postgres", postgres).WithExposedPort(8080).
		AsService(dagger.ContainerAsServiceOpts{Args: []string{"/app/server", "--noUpdates"}})
	tunnel, err := client.Host().Tunnel(server).Start(ctx)
	if err != nil {
		return err
	}
	defer tunnel.Stop(context.WithoutCancel(ctx))
	endpoint, err := tunnel.Endpoint(ctx, dagger.ServiceEndpointOpts{Scheme: "http"})
	if err != nil {
		return err
	}
	testPath := filepath.Join(projectRoot(), "dist", "windows-tests", "windows-bundle.test.exe")
	if err := os.MkdirAll(filepath.Dir(testPath), 0o755); err != nil {
		return err
	}
	_, err = goContainer(client, src).WithEnvVariable("GOOS", "windows").WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{"go", "test", "-c", "-tags=integration", "-o", "/e2e.exe", "./integration"}).File("/e2e.exe").Export(ctx, testPath)
	if err != nil {
		return err
	}
	toWindows := func(path string) (string, error) {
		out, err := exec.CommandContext(ctx, "wslpath", "-w", path).Output()
		return strings.TrimSpace(string(out)), err
	}
	script, err := toWindows(filepath.Join(projectRoot(), "scripts", "windows", "test-worker.ps1"))
	if err != nil {
		return err
	}
	testBinary, err := toWindows(testPath)
	if err != nil {
		return err
	}
	if desktopService := os.Getenv("WINDOWS_DESKTOP_SERVICE"); desktopService != "" {
		desktopScript, err := toWindows(filepath.Join(projectRoot(), "scripts", "windows", "test-desktop-worker.ps1"))
		if err != nil {
			return err
		}
		worker := os.Getenv("WINDOWS_DESKTOP_WORKER")
		if worker == "" {
			return fmt.Errorf("WINDOWS_DESKTOP_WORKER is required")
		}
		return runCmd(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", desktopScript, "-Bundle", bundle, "-TestBinary", testBinary, "-ServerURL", endpoint, "-ServiceBinary", desktopService, "-WorkerBinary", worker)
	}
	return runCmd(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-Bundle", bundle, "-TestBinary", testBinary, "-ServerURL", endpoint, "-Codec", envOrDefault("WINDOWS_CODEC", "libsvtav1"))
}

// WindowsInstaller packages the complete managed Windows installation.
func (Build) WindowsInstaller(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	src := projectSource(client)
	tools, err := windowsWorkerTools(ctx, client)
	if err != nil {
		return err
	}
	return exportWindowsInstaller(ctx, client, src, tools)
}

func exportWindowsInstaller(ctx context.Context, client *dagger.Client, src *dagger.Directory, tools *dagger.Container) error {
	builder, setup := windowsInstallerBuild(client, src, tools.Directory("/bundle"), projectVersion, "", false)
	for _, entry := range []struct{ source, target string }{
		{"/stage/transcoderd-desktop-service.exe", "transcoderd-desktop-service.exe"},
		{"/stage/transcoderd-updater.exe", "transcoderd-updater.exe"},
		{"/windows-installation-files.json", "windows-installation-files.json"},
		{"/" + setup, setup},
	} {
		if _, err := builder.File(entry.source).Export(ctx, filepath.Join(projectRoot(), "dist", entry.target)); err != nil {
			return err
		}
	}
	tool := windowsGoBinary(client, src, "./cmd/windows-update-manifest", "windows-update-manifest", projectVersion, "", "windows")
	_, err := tool.Export(ctx, filepath.Join(projectRoot(), "dist", "windows-update-manifest.exe"))
	return err
}

// WindowsDistribution prepares the complete portable and desktop packages and
// the native test executables from the same Dagger session.
func (Build) WindowsDistribution(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	source := projectSource(client)
	tools, err := windowsWorkerTools(ctx, client)
	if err != nil {
		return err
	}
	binary := windowsWorkerBinary(client, source)
	dist := filepath.Join(projectRoot(), "dist")
	builds, buildCtx := errgroup.WithContext(ctx)
	builds.SetLimit(2)
	builds.Go(func() error {
		archive, name := windowsPortableArchive(tools, binary)
		if _, err := archive.Export(buildCtx, filepath.Join(dist, name)); err != nil {
			return fmt.Errorf("export Windows portable ZIP: %w", err)
		}
		_, err := binary.Export(buildCtx, filepath.Join(dist, "transcoderd-worker-windows-amd64.exe"))
		return err
	})
	builds.Go(func() error {
		return exportWindowsInstaller(buildCtx, client, source, tools)
	})
	builds.Go(func() error {
		_, err := windowsTestBinaries(client, source).Export(buildCtx, filepath.Join(dist, "windows-tests"))
		return err
	})
	return builds.Wait()
}
