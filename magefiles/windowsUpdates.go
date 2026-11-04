package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dagger.io/dagger"
)

func windowsGoBinary(client *dagger.Client, source *dagger.Directory, pkg, name, version, extra, goos string) *dagger.File {
	flags := strings.Replace(ldflags(name), "transcoder/version.Version="+projectVersion, "transcoder/version.Version="+version, 1) + " " + extra
	return goContainer(client, source).WithEnvVariable("GOOS", goos).WithEnvVariable("GOARCH", "amd64").WithExec([]string{"go", "build", "-ldflags", flags, "-o", "/output/app", pkg}).File("/output/app")
}

func windowsDesktopUI(client *dagger.Client) *dagger.Directory {
	return client.Container().From("mcr.microsoft.com/dotnet/sdk:8.0.427").WithDirectory("/src", client.Host().Directory(filepath.Join(projectRoot(), "windows", "desktop"), dagger.HostDirectoryOpts{Exclude: []string{"bin", "obj", "publish"}})).WithDirectory("/worker/dashboard/wwwroot", client.Host().Directory(filepath.Join(projectRoot(), "worker", "dashboard", "wwwroot"))).WithWorkdir("/src").WithExec([]string{"dotnet", "publish", "-c", "Release", "-r", "win-x64", "--self-contained", "true", "-o", "/out"}).Directory("/out")
}

func windowsInstallerBuild(ctx context.Context, client *dagger.Client, source *dagger.Directory, version, extra string, fixture bool) (*dagger.Container, string, error) {
	worker := windowsGoBinary(client, source, "./worker", "transcoderd-worker", version, extra, "windows")
	service := windowsGoBinary(client, source, "./cmd/windows-service", "transcoderd-desktop-service", version, extra, "windows")
	updater := windowsGoBinary(client, source, "./cmd/windows-updater", "transcoderd-updater", version, extra, "windows")
	tool := windowsGoBinary(client, source, "./cmd/windows-update-manifest", "windows-update-manifest", version, "", "linux")
	workerTools, err := windowsWorkerTools(ctx, client)
	if err != nil {
		return nil, "", err
	}
	bundle := workerTools.Directory("/bundle")
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
	return builder.WithExec(args), setup, nil
}

// WindowsUpdateFixtures builds signed-feed-only installers for isolated CI.
func (Test) WindowsUpdateFixtures(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	// Public test-only trust: stable fixtures let Dagger reuse installer layers
	// on retries. Production binaries trust the separate pinned release key.
	seed := sha256.Sum256([]byte("TranscoderD isolated Windows update fixtures v1"))
	key := ed25519.NewKeyFromSeed(seed[:])
	pub := key.Public().(ed25519.PublicKey)
	dir := filepath.Join(projectRoot(), "dist", "windows-update-fixtures")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "key"), []byte(base64.StdEncoding.EncodeToString(key)), 0600); err != nil {
		return err
	}
	extra := "-X transcoder/update/managed.FixtureMode=true -X transcoder/update/managed.FixtureKey=" + base64.StdEncoding.EncodeToString(pub) + " -X transcoder/update/managed.FixtureEndpoint=http://127.0.0.1:18781/latest"
	source := projectSource(client)
	var builds sync.WaitGroup
	results := make(chan error, 2)
	for _, version := range []string{"v99.0.0", "v99.1.0"} {
		builds.Go(func() {
			builder, setup, err := windowsInstallerBuild(ctx, client, source, version, extra, true)
			if err != nil {
				results <- err
				return
			}
			for _, entry := range []struct{ source, name string }{{"/" + setup, setup}, {"/windows-installation-files.json", "files.json"}} {
				if _, err := builder.File(entry.source).Export(ctx, filepath.Join(dir, version, entry.name)); err != nil {
					results <- err
					return
				}
			}
		})
	}
	builds.Wait()
	close(results)
	for err := range results {
		return err
	}
	for _, entry := range []struct{ pkg, name string }{{"./cmd/windows-update-manifest", "sign.exe"}, {"./cmd/windows-update-fixture", "feed.exe"}} {
		binary := windowsGoBinary(client, source, entry.pkg, "fixture", "v99.0.0", extra, "windows")
		if _, err = binary.Export(ctx, filepath.Join(dir, entry.name)); err != nil {
			return err
		}
	}
	return nil
}
