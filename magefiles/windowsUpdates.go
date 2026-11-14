package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dagger.io/dagger"
	"golang.org/x/sync/errgroup"
)

func windowsGoBinary(client *dagger.Client, source *dagger.Directory, pkg, name, version, extra, goos string) *dagger.File {
	flags := strings.Replace(ldflags(name), "transcoder/version.Version="+projectVersion, "transcoder/version.Version="+version, 1) + " " + extra
	return goContainer(client, source).WithEnvVariable("GOOS", goos).WithEnvVariable("GOARCH", "amd64").WithExec([]string{"go", "build", "-ldflags", flags, "-o", "/output/app", pkg}).File("/output/app")
}

func windowsDesktopUI(client *dagger.Client) *dagger.Directory {
	return client.Container().From("mcr.microsoft.com/dotnet/sdk:8.0.427").WithDirectory("/src", client.Host().Directory(filepath.Join(projectRoot(), "windows", "desktop"), dagger.HostDirectoryOpts{Exclude: []string{"bin", "obj", "publish"}})).WithDirectory("/worker/dashboard/wwwroot", client.Host().Directory(filepath.Join(projectRoot(), "worker", "dashboard", "wwwroot"))).WithWorkdir("/src").WithExec([]string{"dotnet", "publish", "-c", "Release", "-r", "win-x64", "--self-contained", "true", "-o", "/out"}).Directory("/out")
}

func windowsInstallerBuild(client *dagger.Client, source, bundle *dagger.Directory, version, extra string, fixture bool) (*dagger.Container, string) {
	worker := windowsGoBinary(client, source, "./worker", "transcoderd-worker", version, extra, "windows")
	service := windowsGoBinary(client, source, "./cmd/windows-service", "transcoderd-desktop-service", version, extra, "windows")
	updater := windowsGoBinary(client, source, "./cmd/windows-updater", "transcoderd-updater", version, extra, "windows")
	tool := windowsGoBinary(client, source, "./cmd/windows-update-manifest", "windows-update-manifest", version, "", "linux")
	setup := "transcoderd-worker-" + version + "-windows-amd64-setup.exe"
	base := client.Container().From("debian:trixie-slim").
		WithExec([]string{"bash", "-c", "apt-get update && apt-get install -y --no-install-recommends nsis curl ca-certificates && rm -rf /var/lib/apt/lists/*"})
	// Pinned downloads depend only on their recipe, never the application
	// commit or fixture version. All three installers share this cache entry.
	prerequisites := base.WithExec([]string{"bash", "-c", `set -eu; mkdir -p /prerequisites; curl -fLsS --retry 5 'https://download.visualstudio.microsoft.com/download/pr/990c720a-46ce-4dc7-87b9-c2eb615d34b9/CC0FF0EB1DC3F5188AE6300FAEF32BF5BEEBA4BDD6E8E445A9184072096B713B/VC_redist.x64.exe' -o /prerequisites/vc_redist.x64.exe; echo 'cc0ff0eb1dc3f5188ae6300faef32bf5beeba4bdd6e8e445a9184072096b713b  /prerequisites/vc_redist.x64.exe' | sha256sum -c; curl -fLsS --retry 5 'https://msedge.sf.dl.delivery.mp.microsoft.com/filestreamingservice/files/4a41d80d-4493-4463-a466-d0988b97f1c4/MicrosoftEdgeWebview2Setup.exe' -o /prerequisites/MicrosoftEdgeWebview2Setup.exe; echo 'ca4c9016c70e2184c5e231662e0e98abf4826647180ef3faab32fcb1ca2ec15e  /prerequisites/MicrosoftEdgeWebview2Setup.exe' | sha256sum -c`}).Directory("/prerequisites")
	builder := base.WithDirectory("/stage", bundle).
		WithDirectory("/stage", windowsDesktopUI(client)).
		WithDirectory("/stage/prerequisites", prerequisites).
		WithFile("/stage/transcoderd-worker.exe", worker).
		WithFile("/stage/transcoderd-desktop-service.exe", service).
		WithFile("/stage/transcoderd-updater.exe", updater).
		WithFile("/manifest", tool).
		WithFile("/installer.nsi", client.Host().File(filepath.Join(projectRoot(), "windows", "installer", "installer.nsi"))).
		WithExec([]string{"/manifest", "files", "/stage", "/windows-installation-files.json"})
	args := []string{"makensis", "-DVERSION=" + version, "-DINPUT=/stage", "-DOUTPUT=/" + setup}
	if fixture {
		args = append(args, "-DFIXTURE=1")
	}
	args = append(args, "/installer.nsi")
	return builder.WithExec(args), setup
}

// WindowsUpdateFixtures exports complete fixtures for local and release validation.
func (Test) WindowsUpdateFixtures(ctx context.Context) error {
	return exportWindowsUpdateFixtures(ctx, false, false)
}

// WindowsUpdateCI exports an independent native update test artifact. PRs
// only need the OCR languages used by integration/testdata/e2e_fixture.mkv.
func (Test) WindowsUpdateCI(ctx context.Context) error {
	return exportWindowsUpdateFixtures(ctx, os.Getenv("WINDOWS_FULL_UPDATE_PAYLOAD") != "true", true)
}

func exportWindowsUpdateFixtures(ctx context.Context, focused, harness bool) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	tools, err := windowsWorkerTools(ctx, client)
	if err != nil {
		return err
	}
	bundle := tools.Directory("/bundle")
	if focused {
		ocr := bundle.Directory("tessdata")
		bundle = bundle.WithoutDirectory("tessdata").
			WithFile("tessdata/eng.traineddata", ocr.File("eng.traineddata")).
			WithFile("tessdata/spa.traineddata", ocr.File("spa.traineddata"))
	}

	// The deterministic fixture trust root is public. The Windows release
	// signing key is never needed to construct or run these installers.
	seed := sha256.Sum256([]byte("TranscoderD isolated Windows update fixtures v1"))
	key := ed25519.NewKeyFromSeed(seed[:])
	pub := key.Public().(ed25519.PublicKey)
	dir := filepath.Join(projectRoot(), "dist", "windows-update-fixtures")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte(base64.StdEncoding.EncodeToString(key)), 0600); err != nil {
		return err
	}
	extra := "-X transcoder/update/managed.FixtureMode=true -X transcoder/update/managed.FixtureKey=" + base64.StdEncoding.EncodeToString(pub) + " -X transcoder/update/managed.FixtureEndpoint=http://127.0.0.1:18781/latest"
	source := projectSource(client)
	builds, buildCtx := errgroup.WithContext(ctx)
	builds.SetLimit(2)
	for _, version := range []string{"v99.0.0", "v99.1.0"} {
		builds.Go(func() error {
			builder, setup := windowsInstallerBuild(client, source, bundle, version, extra, true)
			files := client.Directory().
				WithFile(setup, builder.File("/"+setup)).
				WithFile("files.json", builder.File("/windows-installation-files.json"))
			if _, err := files.Export(buildCtx, filepath.Join(dir, version)); err != nil {
				return fmt.Errorf("export update fixture %s: %w", version, err)
			}
			return nil
		})
	}
	builds.Go(func() error {
		files := client.Directory().
			WithFile("sign.exe", windowsGoBinary(client, source, "./cmd/windows-update-manifest", "fixture", "v99.0.0", extra, "windows")).
			WithFile("feed.exe", windowsGoBinary(client, source, "./cmd/windows-update-fixture", "fixture", "v99.0.0", extra, "windows"))
		if _, err := files.Export(buildCtx, dir); err != nil {
			return err
		}
		if harness {
			return exportWindowsUpdateHarness(buildCtx, client, source, tools)
		}
		return nil
	})
	return builds.Wait()
}

func exportWindowsUpdateHarness(ctx context.Context, client *dagger.Client, source *dagger.Directory, tools *dagger.Container) error {
	binaries := windowsE2eBinaries(client, source)
	input := client.Directory().
		WithFile("bin/ffprobe.exe", tools.File("/bundle/bin/ffprobe.exe")).
		WithFile("windows-bundle.test.exe", binaries.File("/output/windows-bundle.test.exe")).
		WithFile("windows-e2e-server.exe", binaries.File("/output/windows-e2e-server.exe"))
	_, err := input.Export(ctx, filepath.Join(projectRoot(), "dist", "windows-update-harness"))
	return err
}
