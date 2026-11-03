## Why

Windows CI currently executes native units and bundled-tool checks but does not
exercise the worker's complete HTTP workflow. Hosted Windows runners need a CPU
E2E so release artifacts are tested without an NVIDIA GPU.

## What Changes

- Cross-build a Windows server executable for the test harness using Dagger.
- Run an isolated native server/PostgreSQL/worker fixture E2E with SVT-AV1 on the
  Windows runner, preserving logs and cleaning up owned processes.
- Require native units, media/OCR checks and this CPU E2E before uploading the
  Windows ZIP and platform executable to a release.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `distribution`: native Windows CPU E2E and validated Windows release assets.

## Impact

The product worker and its defaults do not change. CI gains a test-only Windows
server binary and uses the PostgreSQL binaries provided by windows-2025. The
Linux Dagger build remains responsible for producing Windows artifacts. Local
Linux-server/Windows-worker GPU validation remains an optional hardware check.
