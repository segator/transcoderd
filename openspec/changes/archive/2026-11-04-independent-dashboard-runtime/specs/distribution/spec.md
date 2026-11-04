## MODIFIED Requirements

### Requirement: Separate Go binaries
The build SHALL produce `transcoderd-server`, `transcoderd-worker` and `transcoderd-frontend` executables with application name, version, commit, and build date injected at build time.

#### Scenario: Build both binaries
- **WHEN** the Go build target for all applications runs
- **THEN** `dist/transcoderd-server`, `dist/transcoderd-worker` and `dist/transcoderd-frontend` are produced

#### Scenario: Request binary version
- **WHEN** any application binary is run with `--version`
- **THEN** it prints version information and exits

### Requirement: Separate container targets
The Docker build SHALL expose `server`, `worker` and `frontend` targets with their corresponding binaries as entry points. The worker image SHALL include FFmpeg, ffprobe, mkvextract, .NET, PgsToSrt, and Tesseract data required by the media pipeline.

#### Scenario: Server image
- **WHEN** the `server` target is built
- **THEN** its entry point runs `transcoderd-server`

#### Scenario: Worker image
- **WHEN** the `worker` target is built
- **THEN** its entry point runs `transcoderd-worker` with the bundled media tools available

#### Scenario: Frontend image
- **WHEN** the frontend target is built
- **THEN** its entry point runs transcoderd-frontend independently with CA certificates and no media tools or media/database mounts

### Requirement: Versioned image tags
Publishing SHALL tag server, worker and frontend images with their role and requested version under `ghcr.io/segator/transcoderd`.

#### Scenario: Versioned publish
- **WHEN** all application images are published for a version
- **THEN** `server-<version>`, `worker-<version>` and `frontend-<version>` tags are available

#### Scenario: Release aliases retain the canonical application version
- **WHEN** a release image is published under full, minor, and major version tags
- **THEN** all tags for each role resolve to the same image artifact built with the full semantic release version
