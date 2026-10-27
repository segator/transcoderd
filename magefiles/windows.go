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

// Windows builds the native Windows x64 worker and its portable tool bundle.
func (Build) Windows(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	binary := windowsWorkerBinary(client, projectSource(client))
	if _, err := binary.Export(ctx, filepath.Join(projectRoot(), "dist", "transcoderd-worker-windows-amd64.exe")); err != nil {
		return err
	}
	bundle := windowsWorkerBundle(client, binary)
	archive := "transcoderd-worker-" + projectVersion + "-windows-amd64.zip"
	_, err = bundle.WithWorkdir("/bundle").WithExec([]string{"zip", "-q", "-r", "/" + archive, "."}).File("/"+archive).Export(ctx, filepath.Join(projectRoot(), "dist", archive))
	return err
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
	return goContainer(client, src).WithEnvVariable("GOOS", "windows").WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{"go", "build", "-ldflags", ldflags("transcoderd-worker"), "-o", "/output/worker.exe", "./worker"}).File("/output/worker.exe")
}

// WindowsBinaries exports native Windows unit-test executables. Run them with
// scripts/windows/run-tests.ps1 on Windows; cross-compilation is not execution.
func (Test) WindowsBinaries(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = goContainer(client, projectSource(client)).
		WithEnvVariable("GOOS", "windows").WithEnvVariable("GOARCH", "amd64").
		WithExec([]string{"bash", "-c", `set -eu; mkdir -p /output; for pkg in $(go list ./cmd/... ./helper/... ./model/... ./update/... ./version/... ./worker/...); do name=$(echo "$pkg" | tr / _); go test -c -o "/output/$name.test.exe" "$pkg"; done`}).
		WithExec([]string{"go", "test", "-c", "-tags=integration", "-o", "/output/windows-bundle.test.exe", "./integration"}).
		WithExec([]string{"go", "build", "-ldflags", ldflags("transcoderd-server"), "-o", "/output/windows-e2e-server.exe", "./server"}).
		Directory("/output").Export(ctx, filepath.Join(projectRoot(), "dist", "windows-tests"))
	return err
}

func windowsFFmpeg(client *dagger.Client) *dagger.Container {
	container := client.Container().From("debian:trixie-slim").
		WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates curl xz-utils make nasm cmake ninja-build pkg-config git gcc-mingw-w64-x86-64-posix g++-mingw-w64-x86-64-posix && rm -rf /var/lib/apt/lists/*"}).
		WithEnvVariable("BUILD_JOBS", envOrDefault("WINDOWS_BUILD_JOBS", "2")).
		WithFile("/build-ffmpeg.sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "windows", "build-ffmpeg.sh")))
	for _, stage := range []string{"aom", "svt", "x265", "fdk", "nvcodec", "ffmpeg"} {
		if stage == "ffmpeg" {
			// FFmpeg executes native table generators as part of a cross-build.
			container = container.WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*"})
		}
		container = container.WithFile("/"+stage+".sh", client.Host().File(filepath.Join(projectRoot(), "scripts", "windows", stage+".sh"))).
			WithExec([]string{"bash", "/build-ffmpeg.sh", stage})
	}
	return container
}

func windowsPGS(client *dagger.Client) *dagger.Container {
	return pgsBuilderImage(client).WithExec([]string{"bash", "-c", fmt.Sprintf("cd /src/PgsToSrt-%s/src && dotnet publish PgsToSrt/PgsToSrt.csproj -c Release -f net8.0 -r win-x64 --self-contained true -o /windows-pgs", pgstosrtVersion)})
}

func windowsWorkerBundle(client *dagger.Client, binary *dagger.File) *dagger.Container {
	ffmpeg := windowsFFmpeg(client)
	pgs := windowsPGS(client)
	return client.Container().From("debian:trixie-slim").
		WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates curl 7zip zip && rm -rf /var/lib/apt/lists/*"}).
		WithExec([]string{"bash", "-c", fmt.Sprintf("curl --fail --location --retry 5 https://mkvtoolnix.download/windows/releases/103.0/mkvtoolnix-64-bit-103.0.7z -o /mkv.7z && echo '%s  /mkv.7z' | sha256sum --check --status && 7z x /mkv.7z -o/mkv && rm /mkv.7z", windowsMKVSHA256)}).
		WithDirectory("/bundle", ffmpeg.Directory("/output")).
		WithFile("/bundle/transcoderd-worker.exe", binary).
		WithDirectory("/bundle/pgs", pgs.Directory("/windows-pgs")).
		WithDirectory("/bundle/tessdata", pgs.Directory("/src/tessdata")).
		WithFile("/bundle/licenses/TranscoderD.txt", client.Host().File(filepath.Join(projectRoot(), "LICENSE"))).
		WithNewFile("/bundle/licenses/PgsToSrt-source.txt", "PgsToSrt source: https://github.com/Tentacule/PgsToSrt/tree/"+pgstosrtVersion+"\nLocal patch: pgs-patches/pgstosrt-bitmap-bounds.patch\n").
		WithExec([]string{"bash", "-c", "cp -a /mkv/mkvtoolnix/mkvextract.exe /bundle/bin/ && find /mkv/mkvtoolnix -maxdepth 1 -iname '*.dll' -exec cp -t /bundle/bin/ {} + && cp -a /mkv/mkvtoolnix/doc/COPYING.txt /bundle/licenses/mkvtoolnix.txt"}).
		WithFile("/bundle/README.md", client.Host().File(filepath.Join(projectRoot(), "docs", "worker-windows.md"))).
		WithFile("/bundle/windows-worker-validation.md", client.Host().File(filepath.Join(projectRoot(), "docs", "windows-worker-validation.md")))
}

const windowsMKVSHA256 = "01d76dbfdbd5615d9e02f69b0710d1429a902d47a98c8eee33bd884c4fcb3cbf"

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

// WindowsInstaller packages the worker, background service and web tray UI.
func (Build) WindowsInstaller(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	source := projectSource(client)
	worker := windowsWorkerBinary(client, source)
	service := goContainer(client, source).WithEnvVariable("GOOS", "windows").WithEnvVariable("GOARCH", "amd64").WithExec([]string{"go", "build", "-o", "/out/transcoderd-desktop-service.exe", "./cmd/windows-service"}).File("/out/transcoderd-desktop-service.exe")
	ui := client.Container().From("mcr.microsoft.com/dotnet/sdk:8.0.427").WithDirectory("/src", client.Host().Directory(filepath.Join(projectRoot(), "windows", "desktop"), dagger.HostDirectoryOpts{Exclude: []string{"bin", "obj", "publish"}})).WithWorkdir("/src").WithExec([]string{"dotnet", "publish", "-c", "Release", "-r", "win-x64", "--self-contained", "true", "-o", "/out"}).Directory("/out")
	if _, err := service.Export(ctx, filepath.Join(projectRoot(), "dist", "transcoderd-desktop-service.exe")); err != nil {
		return err
	}
	bundle := windowsWorkerBundle(client, worker).Directory("/bundle")
	setup := "transcoderd-worker-" + projectVersion + "-windows-amd64-setup.exe"
	builder := client.Container().From("debian:trixie-slim").WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends nsis curl ca-certificates && rm -rf /var/lib/apt/lists/*"}).WithDirectory("/stage", bundle).WithDirectory("/stage", ui).WithFile("/stage/transcoderd-desktop-service.exe", service).WithFile("/installer.nsi", client.Host().File(filepath.Join(projectRoot(), "windows", "installer", "installer.nsi"))).WithExec([]string{"bash", "-c", `mkdir -p /stage/prerequisites; curl -fLsS --retry 5 'https://download.visualstudio.microsoft.com/download/pr/990c720a-46ce-4dc7-87b9-c2eb615d34b9/CC0FF0EB1DC3F5188AE6300FAEF32BF5BEEBA4BDD6E8E445A9184072096B713B/VC_redist.x64.exe' -o /stage/prerequisites/vc_redist.x64.exe; echo 'cc0ff0eb1dc3f5188ae6300faef32bf5beeba4bdd6e8e445a9184072096b713b  /stage/prerequisites/vc_redist.x64.exe' | sha256sum -c; curl -fLsS --retry 5 'https://msedge.sf.dl.delivery.mp.microsoft.com/filestreamingservice/files/4a41d80d-4493-4463-a466-d0988b97f1c4/MicrosoftEdgeWebview2Setup.exe' -o /stage/prerequisites/MicrosoftEdgeWebview2Setup.exe; echo 'ca4c9016c70e2184c5e231662e0e98abf4826647180ef3faab32fcb1ca2ec15e  /stage/prerequisites/MicrosoftEdgeWebview2Setup.exe' | sha256sum -c`}).WithExec([]string{"makensis", "-DVERSION=" + projectVersion, "-DINPUT=/stage", "-DOUTPUT=/" + setup, "/installer.nsi"})
	_, err = builder.File("/"+setup).Export(ctx, filepath.Join(projectRoot(), "dist", setup))
	return err
}
