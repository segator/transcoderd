package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"dagger.io/dagger"
)

// Web runs frontend PWA/resource and worker activity tests in native Node 24.
func (Test) Web(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	source := client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Include: []string{"frontend/web/**", "worker/dashboard/**"},
	})
	_, err = client.Container().From("node:24-trixie-slim").
		WithDirectory("/src", source).WithWorkdir("/src").
		WithExec([]string{"node", "--test", "frontend/web/dashboardPWA_test.cjs", "frontend/web/resources_test.cjs", "worker/dashboard/activity_test.js"}).Sync(ctx)
	return err
}

// Tooling runs the Magefile unit suites on the shared Dagger Engine.
func (Test) Tooling(ctx context.Context) error {
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	source := client.Host().Directory(projectRoot(), dagger.HostDirectoryOpts{
		Include: []string{"magefiles/*.go", "magefiles/go.mod", "magefiles/go.sum", "scripts/windows/fetch.sh"},
	})
	_, err = goContainer(client, source).
		WithWorkdir("/src/magefiles").
		WithExec([]string{"go", "test", "./..."}).
		Sync(ctx)
	return err
}

// Source validates release guards and appends immutable source outputs for CI.
// The current CI-tool executable survives a later checkout of an older tag.
func (Release) Source(ctx context.Context) error {
	output := os.Getenv("GITHUB_OUTPUT")
	if output == "" {
		return fmt.Errorf("GITHUB_OUTPUT is required to resolve CI source")
	}
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		return fmt.Errorf("GH_TOKEN is required to validate tagged release source")
	}
	client, err := daggerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	source := client.Host().Directory(filepath.Join(projectRoot(), "scripts", "release"), dagger.HostDirectoryOpts{Exclude: []string{"__pycache__"}})
	tool := releaseTools(client).WithDirectory("/release", source).WithWorkdir("/release").
		WithExec([]string{"python3", "-m", "unittest", "discover", "-v"})
	for _, name := range []string{"GITHUB_EVENT_NAME", "GITHUB_REF", "GITHUB_SHA", "GITHUB_REPOSITORY", "RELEASE_TAG"} {
		tool = tool.WithEnvVariable(name, os.Getenv(name))
	}
	resolved, err := tool.WithSecretVariable("GH_TOKEN", client.SetSecret("source-github-token", token)).
		WithEnvVariable("GITHUB_OUTPUT", "/source.outputs").
		// API checks must be fresh even on a retry with identical tagged source.
		WithEnvVariable("SOURCE_CHECK_RUN", time.Now().UTC().Format(time.RFC3339Nano)).
		WithExec([]string{"python3", "resolve_release.py"}).File("/source.outputs").Contents(ctx)
	if err != nil {
		return fmt.Errorf("resolve CI source: %w", err)
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.WriteString(file, resolved)
	return err
}

func releaseTools(client *dagger.Client) *dagger.Container {
	return client.Container().From("debian:trixie-slim").
		WithExec([]string{"sh", "-c", "apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tar python3 && rm -rf /var/lib/apt/lists/*"}).
		WithExec([]string{"sh", "-c", fmt.Sprintf(
			`set -eu; gh_arch=$(dpkg --print-architecture); curl -fsSL "https://github.com/cli/cli/releases/download/v%s/gh_%s_linux_${gh_arch}.tar.gz" -o /tmp/gh.tar.gz; tar -xzf /tmp/gh.tar.gz --strip-components=2 -C /usr/local/bin "gh_%s_linux_${gh_arch}/bin/gh"; gh --version | grep -Fq 'gh version %s'; rm /tmp/gh.tar.gz`,
			githubCLIVersion, githubCLIVersion, githubCLIVersion, githubCLIVersion,
		)})
}
