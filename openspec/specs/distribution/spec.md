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

### Requirement: AV1-capable FFmpeg in the worker image
The worker image SHALL bundle FFmpeg 9.0.2 with `libsvtav1`, `libaom-av1`, and `av1_nvenc` available. It SHALL retain `libx265` for explicit HEVC rollback and `libfdk_aac` for the default audio setting. When a compatible NVIDIA GPU and driver are exposed to the worker container, its bundled FFmpeg SHALL be able to encode 10-bit AV1 with the worker's default `av1_nvenc` video profile.

#### Scenario: Inspect bundled FFmpeg
- **WHEN** the worker image is built and its FFmpeg encoder list is inspected
- **THEN** FFmpeg reports version 9.0.2 and lists `libsvtav1`, `libaom-av1`, `av1_nvenc`, `libx265`, and `libfdk_aac`

#### Scenario: Encode AV1 on an exposed NVIDIA GPU
- **WHEN** the worker image runs with a compatible NVIDIA GPU and driver
- **THEN** its FFmpeg completes a 10-bit `av1_nvenc` encode using the default worker video profile
