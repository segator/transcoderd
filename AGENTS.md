# AGENTS.md — Coding Agent Guidelines for TranscoderD

## Project Overview

TranscoderD is a distributed video transcoding system (server/worker architecture) written in **Go 1.26**.
Module name: `transcoder`. License: GPLv3.

## OpenSpec

- The current behavior contract lives in [`openspec/specs/`](openspec/specs/). Read the relevant capability before changing observable behavior. [`openspec/BASELINE.md`](openspec/BASELINE.md) maps capabilities to code and records known implementation gaps. [`openspec/CONFIGURATION.md`](openspec/CONFIGURATION.md) and [`openspec/PROTOCOL.md`](openspec/PROTOCOL.md) give the current settings and HTTP wire format.
- Use the generated OpenSpec skills in `.agents/skills/` for changes: explore or propose, review the proposal and delta specs, apply, check the result against the specs, then archive. Keep active deltas in `openspec/changes/<change>/specs/`; archive completed changes so `openspec/specs/` reflects the shipped behavior.
- Add or modify a requirement only for an observable contract change. Document configuration defaults and output effects in the relevant spec; keep exact command construction and internal control flow in design/tasks or reference docs. Preserve each capability's existing path. Write all OpenSpec artifacts in English, retaining the `SHALL`/`MUST` markers.
- When implementation and a spec disagree, investigate the code and tests, record the discrepancy in `openspec/BASELINE.md`, and resolve it through a change. Do not silently rewrite a spec to hide a regression.
- Validate spec changes with `openspec validate --all --strict` (or `npx -y @fission-ai/openspec@latest validate --all --strict` when the CLI is not installed). Run relevant Go tests as well.

## Build & Run Commands

```bash
mage build:all          # Build server and worker binaries via Dagger
mage build:server       # Build only the server binary
mage build:worker       # Build only the worker binary
mage docker:all         # Build server and worker images via Dagger
mage docker:server      # Build only the server image
mage docker:worker      # Build only the worker image
mage lint:fmt           # Format Go source
mage lint:check         # Run golangci-lint in Dagger
mage lint:fix           # Apply golangci-lint fixes in Dagger
```

## Test Commands

```bash
mage test:unit          # Unit tests with coverage
mage test:race          # Unit tests with race detector
mage test:short         # Unit tests in short mode
mage test:integration   # Integration tests via Dagger services
mage test:e2e           # Docker end-to-end test via Dagger
mage test:all           # Unit + integration tests

# Run a single test by name
go test -v -run TestFunctionName ./path/to/package/...

# Run a single test file's tests (by package)
go test -v ./server/scheduler/...

# Run integration tests (separated by build tag)
go test -tags=integration -v -timeout 5m ./integration/...
```

## CI Pipeline

- **Lint**: `mage lint:check` via `.github/workflows/lint.yml` (Go 1.26)
- **Build+Test+E2E**: Mage targets backed by Dagger via `.github/workflows/main.yml`
- **Release**: Release Please (conventional commits) on push to `main`
- Dev environment managed by **mise**: Go 1.26.0, Mage, and Dagger

## Code Style Guidelines

### Imports

- Group imports in blocks separated by blank lines: stdlib, then internal (`transcoder/...`), then third-party.
- Always alias logrus as `log`:
  ```go
  log "github.com/sirupsen/logrus"
  ```

### Formatting

- Use `gofmt` (enforced by golangci-lint).
- Linter config: `.golangci.yaml` — enables `gocritic` linter and `gofmt` formatter.
- Excluded presets: `comments`, `common-false-positives`, `legacy`, `std-error-handling`.

### Error Handling

- Define sentinel errors at package level with `var` and `errors.New`:
  ```go
  var ErrNoJobsAvailable = errors.New("no jobs available to process")
  ```
- Prefer `Err` prefix for sentinel error names (e.g., `ErrElementNotFound`).
- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Check sentinel errors with `errors.Is(err, ...)`.
- Use `log.Panic` or `log.Fatal` only in `main()` for unrecoverable startup failures.
- Lower-level code returns errors upward; do not panic in library code.

### Naming Conventions

- **Packages**: single lowercase words (`model`, `scheduler`, `repository`, `step`).
- **Files**: lowerCamelCase for multi-word (`downloadStep.go`, `stepExecutor.go`); test files use `*_test.go`.
- **Types**: PascalCase (`RuntimeScheduler`, `SQLRepository`, `JobExecutor`).
- **Interfaces**: placed near the consumer; keep them small and focused.
- **Constructors**: `New<TypeName>(...)` returning `(*TypeName, error)` or `*TypeName`.
- **Receiver names**: single letter matching the type (`r` for Repository, `s` for Scheduler, `e` for Executor).
- **Constants**: PascalCase for exported (`PingEvent`), camelCase for unexported (`maxActiveJobs`).
- **Enum types**: string-based type with `const` block:
  ```go
  type EventType string
  const (
      PingEvent         EventType = "Ping"
      NotificationEvent EventType = "Notification"
  )
  ```

### Logging

- Logger: `github.com/sirupsen/logrus`, always imported as `log`.
- Use structured logging with `log.WithFields(log.Fields{...})` for business events.
- Field keys use `snake_case` (e.g., `"job_id"`, `"source_path"`, `"worker"`).
- Log levels: `Debug` for internals/SQL, `Info` for lifecycle, `Warn` for degraded state, `Error` for operational errors, `Panic`/`Fatal` only at startup.
- The worker has a separate `console.LeveledLogger` interface for TUI output.

### Configuration

- Three-layer config: **pflag** CLI flags → **Viper** (YAML + env vars with `TR_` prefix) → **mapstructure** struct tags.
- Config structs use `mapstructure:"fieldName"` tags.
- Nested config uses pointer fields: `*scheduler.Config`, `*repository.SQLServerConfig`.
- Config files searched in: `/etc/transcoderd/`, `~/.transcoderd/`, `.` (current dir).

### Type Patterns

- **Functional options** for configurable constructors:
  ```go
  type ExecutorOption func(*Executor)
  func WithParallelRunners(n int) ExecutorOption { ... }
  ```
- **Struct embedding** for composition (`sync.RWMutex`, pointer embedding for stream types).
- **Interfaces**: defined close to consumer, not producer. Keep small.

### Testing

- **Table-driven tests** are the standard pattern:
  ```go
  tests := []struct {
      name string
      input string
      want  bool
  }{ ... }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- Use stdlib assertions only (`t.Errorf`, `t.Fatalf`); no testify assert/require.
- Error format: `"FunctionName() = %v, want %v"`.
- Variable convention: `tests` for the slice, `tt` for each case, `got`/`want` for values.
- **Mocks are hand-written** (no code generation). Place mock files as `*_mock.go`.
- Tests are in the **same package** (white-box testing) to access unexported symbols.
- Integration tests use `//go:build integration` tag and live in `integration/`.
- Integration tests use **testcontainers-go** for real PostgreSQL.
- Use `t.Skip()` for tests needing external dependencies that may not be available.
- Temp dirs: `os.TempDir()` + UUID, cleaned with `defer os.RemoveAll(...)`.

### Project Structure

```
cmd/            # Shared CLI config (pflag, viper, mapstructure)
model/          # Domain types (Job, Event, Worker)
helper/         # Utilities (command exec, concurrent collections, progress)
server/         # Server binary
  config/       # Server config structs
  repository/   # PostgreSQL repository (interface + SQL impl + mock)
  scheduler/    # Job scheduling engine
  web/          # HTTP API (gorilla/mux)
worker/         # Worker binary
  config/       # Worker config structs (FFmpeg, PGS, etc.)
  console/      # TUI rendering and logging
  ffmpeg/       # ffprobe wrapper
  job/          # Job execution context
  serverclient/ # HTTP client to server API
  step/         # Encoding pipeline steps
  worker/       # Worker coordination loop
integration/    # Integration tests (build tag: integration)
version/        # Build-time version info
update/         # GitHub release self-updater
```

### Version Injection

Binaries are built with ldflags injecting version info:
```
-X main.ApplicationName=transcoderd-{server,worker}
-X transcoder/version.Version=...
-X transcoder/version.Commit=...
-X transcoder/version.Date=...
```

### Docker

Multi-stage Dockerfile: `builder-ffmpeg` and `builder-pgs` provide binaries and OCR assets; `base` (Debian) feeds the `server` and `worker` targets.
The FFmpeg builder uses a versioned GHCR tag selected by `FFMPEG_BUILDER_TAG` in the Makefile. Build it locally with `make build-builder-ffmpeg` and publish it with `make publish-builder-ffmpeg` after verification. The current builder compiles FFmpeg 9.0.2 with software AV1 and NVENC encoders. The active `openspec/changes/av1-worker-profiles/` change makes the approved AV1 NVENC profile the worker default and adds SVT-AV1 and x265 choices; read its delta specs and `openspec/CONFIGURATION.md` before changing these settings. The RTX 5080 validation and provisional quality measurements are in `docs/av1-nvenc-validation.md`, and the watched full-film command is in `docs/av1-gpu-full-film-command.md`. Keep `openspec/specs/distribution/` and the FFmpeg configuration reference in sync when changing the image.
Host driver/runtime requirements and separate CPU/GPU worker examples are in `docs/worker-deployment.md`. `docs/encoding-profiles.md` explains the production parameters, `docs/av1-worker-profile-validation.md` records the full server/worker movie test, and `docs/legacy-hevc-encoding.md` retains the historical benchmark command. Keep these operational references consistent with encoder defaults and image build gates.
Images published to `ghcr.io/segator/transcoderd:{server,worker}-{version}`.
