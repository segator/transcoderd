# Distribution

## Purpose

Define the separately built server and worker executables and images, including the media tools supplied to the worker image.

## Requirements

### Requirement: Separate Go binaries
The build SHALL produce `transcoderd-server` and `transcoderd-worker` executables with application name, version, commit, and build date injected at build time.

#### Scenario: Build both binaries
- **WHEN** the Go build target for both applications runs
- **THEN** `dist/transcoderd-server` and `dist/transcoderd-worker` are produced

#### Scenario: Request binary version
- **WHEN** either binary is run with `--version`
- **THEN** it prints version information and exits

### Requirement: Separate container targets
The Docker build SHALL expose `server` and `worker` targets with their corresponding binaries as entry points. The worker image SHALL include FFmpeg, ffprobe, mkvextract, .NET, PgsToSrt, and Tesseract data required by the media pipeline.

#### Scenario: Server image
- **WHEN** the `server` target is built
- **THEN** its entry point runs `transcoderd-server`

#### Scenario: Worker image
- **WHEN** the `worker` target is built
- **THEN** its entry point runs `transcoderd-worker` with the bundled media tools available

### Requirement: Versioned image tags
Publishing SHALL tag server and worker images with their role and requested version under `ghcr.io/segator/transcoderd`.

#### Scenario: Versioned publish
- **WHEN** both images are published for a version
- **THEN** `server-<version>` and `worker-<version>` tags are available

#### Scenario: Release aliases retain the canonical application version
- **WHEN** a release image is published under full, minor, and major version tags
- **THEN** all tags for each role resolve to the same image artifact built with the full semantic release version

### Requirement: AV1-capable FFmpeg in the worker image
The worker image SHALL bundle FFmpeg 9.0.2 with `libsvtav1`, `libaom-av1`, and `av1_nvenc` available. It SHALL retain `libx265` for explicit HEVC rollback and `libfdk_aac` for the default audio setting. When a compatible NVIDIA GPU and driver are exposed to the worker container, its bundled FFmpeg SHALL be able to encode 10-bit AV1 with the worker's default `av1_nvenc` video profile.

#### Scenario: Inspect bundled FFmpeg
- **WHEN** the worker image is built and its FFmpeg encoder list is inspected
- **THEN** FFmpeg reports version 9.0.2 and lists `libsvtav1`, `libaom-av1`, `av1_nvenc`, `libx265`, and `libfdk_aac`

#### Scenario: Encode AV1 on an exposed NVIDIA GPU
- **WHEN** the worker image runs with a compatible NVIDIA GPU and driver
- **THEN** its FFmpeg completes a 10-bit `av1_nvenc` encode using the default worker video profile

### Requirement: Native Windows worker bundle
The build SHALL produce a Windows x64 worker ZIP containing the worker executable, FFmpeg 9.0.2, ffprobe, mkvextract, patched PgsToSrt, a private .NET runtime, native OCR libraries, and pinned Tesseract language data. Running the bundle SHALL require neither Docker, WSL, Go, nor a globally installed .NET runtime.

#### Scenario: Build the portable worker
- **WHEN** the Windows worker bundle build succeeds
- **THEN** a versioned Windows x64 ZIP and a platform-specific worker executable are available in dist

#### Scenario: Required encoding capabilities
- **WHEN** bundled FFmpeg capabilities are inspected
- **THEN** version 9.0.2 and libsvtav1, libaom-av1, av1_nvenc, libx265, and libfdk_aac are available

#### Scenario: End-to-end Windows processing
- **WHEN** a Windows worker receives the integration fixture from a Linux server
- **THEN** it downloads, extracts and OCRs subtitles, encodes AV1 and AAC, verifies, and uploads a completed output retaining the expected languages

### Requirement: Windows CPU release validation
Windows CI SHALL run native unit tests, bundled media/OCR checks, and a complete SVT-AV1 CPU worker E2E without requiring a GPU. Windows release uploads SHALL occur only after these checks succeed and SHALL attach the versioned portable ZIP and transcoderd-worker-windows-amd64.exe to the matching release.

#### Scenario: Hosted Windows processing
- **WHEN** the Windows CI job runs on a hosted runner without an NVIDIA GPU
- **THEN** the native worker processes the fixture through download, PGS extraction/OCR, SVT-AV1 and AAC encoding, verification, and upload

#### Scenario: Validated Windows release
- **WHEN** a release is created and all Windows checks succeed
- **THEN** its Windows ZIP and platform worker executable are uploaded to that release

#### Scenario: Failed Windows validation
- **WHEN** a native test, bundle check, or Windows CPU E2E fails
- **THEN** the Windows release upload does not run

### Requirement: Windows desktop installer distribution
The release pipeline SHALL build a Windows desktop installer from the same worker and bundled tools as the portable ZIP and SHALL publish it only after native installation, service, desktop control and CPU worker validation succeed.

#### Scenario: Release installer
- **WHEN** a release passes Windows native validation
- **THEN** the release contains a versioned Windows installer in addition to the existing portable assets

#### Scenario: Installer validation fails
- **WHEN** installation, service control, tray smoke test or Windows CPU E2E fails
- **THEN** the Windows installer is not published as a validated release asset

### Requirement: Authenticated Windows installation releases
Validated Windows releases SHALL publish a signed update manifest binding the stable version, x64 installer identity, installer size and digest, and expected installed-file digests. Automatic clients SHALL trust a pinned verification key. The release signing key SHALL remain in protected CI secrets and SHALL NOT be included in application files, artifacts or logs.

#### Scenario: Windows release assets are complete
- **WHEN** native Windows release validation succeeds
- **THEN** the matching installer and signed manifest are published for that version and are discoverable by installed Windows services

#### Scenario: Windows validation fails
- **WHEN** build, native tests or unattended upgrade validation fails
- **THEN** no signed metadata marks that release eligible for automatic Windows installation

### Requirement: Native unattended Windows upgrade validation
Windows CI SHALL validate mandatory unattended upgrades with SVT-AV1 CPU work, original-owner control-channel access, retained settings and post-install health. It SHALL cover immediate admission blocking, automatic retry after failures and restarts, rollback, and invalid metadata rejection in an isolated runner. The validation SHALL require neither a GPU nor manual dialogs.

#### Scenario: Upgrade during an accepted job
- **WHEN** an update is detected during the fixture's accepted work
- **THEN** job requests stop as soon as the release is authenticated, the fixture finishes including upload, and the complete installation upgrades and resumes with its prior limits without user action

#### Scenario: Upgrade fails or is interrupted
- **WHEN** the test injects an installer failure or an incomplete transaction
- **THEN** recovery preserves owner access, configuration and persisted work, retains the pending admission block and retries automatically until the update succeeds

#### Scenario: Invalid update source
- **WHEN** the test supplies a bad signature, digest, version, architecture or incomplete release asset set
- **THEN** the service rejects installation, establishes no pending update from unauthenticated metadata and retains any already authenticated pending update and admission block

#### Scenario: Slow installer download
- **WHEN** the installer download is held open after complete eligible release metadata has been authenticated
- **THEN** the test observes no new worker requests during the download and accepted work continues normally
