# AGENTS.md — Coding Agent Guidelines for TranscoderD

## Project Overview

TranscoderD is a distributed video transcoding system (server/worker architecture) written in **Go 1.24**.
Module name: `transcoder`. License: GPLv3.

## Build & Run Commands

```bash
make build              # Build both Go binaries + Docker containers
make buildgo-server     # Build server binary only → dist/transcoderd-server
make buildgo-worker     # Build worker binary only → dist/transcoderd-worker
make fmt                # Run go fmt ./...
make lint               # Run golangci-lint (gocritic + gofmt)
make lint-fix           # Run golangci-lint --fix
```

## Test Commands

```bash
make test               # Unit tests with coverage
make test-race          # Unit tests with race detector
make test-short         # Unit tests in short mode
make test-integration   # Integration tests (requires Docker for testcontainers)
make test-all           # Unit + integration tests

# Run a single test by name
go test -v -run TestFunctionName ./path/to/package/...

# Run a single test file's tests (by package)
go test -v ./server/scheduler/...

# Run integration tests (separated by build tag)
go test -tags=integration -v -timeout 5m ./integration/...
```

## CI Pipeline

- **Lint**: `golangci-lint` via `.github/workflows/lint.yml` (Go 1.24)
- **Build+Test**: `mage test && mage build` via `.github/workflows/main.yml`
- **Release**: Release Please (conventional commits) on push to `main`

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

Multi-stage Dockerfile: `builder-ffmpeg` → `base` (debian) → `server` / `worker` targets.
Images published to `ghcr.io/segator/transcoderd:{server,worker}-{version}`.
