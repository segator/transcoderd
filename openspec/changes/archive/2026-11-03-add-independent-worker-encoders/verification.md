# Verification

Verified on 2026-11-03 with Go 1.26.10, an isolated PostgreSQL 17 container, and local Chromium.

- `go test -race ./...`: passed. Concurrency tests fill two CPU and three GPU slots simultaneously inside one queue/controller, prevent further starts at capacity, resize each class, disable one class, drain globally paused accepted work, and restore a pinned disabled class with bounded capacity.
- Profile tests validate approved secondary defaults, explicit class-specific YAML overrides, rejection of wrong-class profiles, copied configuration isolation, and job-context disk round trips. A CPU-only accepted assignment arriving after a global pause remains CPU when restored under a GPU-primary startup configuration.
- FFmpeg invocation tests execute a temporary capture program and inspect its complete argv. They verify the pinned CPU/GPU codec, quality, thread setting, and extraArgs override the opposite startup profile without leaking class-specific flags. These tests do not claim physical GPU encoding or compression/quality benchmark results.
- `go test -race -tags=integration -run TestDashboardPostgres ./server/repository`: passed with isolated schema, migrations 007/008, and database timezone Europe/Madrid. Verifies fresh-repository pair persistence, paired dashboard fields, legacy reset, existing aggregate/progress/freshness behavior, and UTC timestamps.
- Existing `TestServerWorkerIntegration`: passed against PostgreSQL, including persisted single-limit compatibility, assignment, synthetic phase events, output finalization, compression sizes, and event history.
- HTTP tests cover atomic pair writes, worker isolation, missing/negative/fractional/mixed/overflow inputs, unchanged values after rejection, modern capability-aware reads, safe aggregate-zero legacy reads, and legacy writes clearing the pair. Existing authentication, CSRF, session and dashboard tests continue passing.
- Chromium checks passed for independent CPU/GPU editing, disabling CPU without changing GPU, global pause, worker isolation, pending acknowledgement, login/logout, charts, job filters/search, period switching, empty/offline/recovery states, and responsive layouts down to 320px. No JavaScript/CSP errors or page overflow. Preview data remains a separate temporary fixture.
- golangci-lint v2.11.4: zero issues. Server and worker binaries compile with the embedded dashboard.
- Strict OpenSpec validation passed before/after main-spec synchronization and is repeated after archival.

No production deployment or physical CPU/GPU benchmark was performed. Deploy the server before updated workers. A GPU target requires a working NVENC runtime; there is no silent CPU fallback. Drain accepted work before downgrading a dual-capable worker because older binaries do not understand per-job profile snapshots.
