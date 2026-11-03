# Verification

Verified on 2026-11-03 using Go 1.26.10 and an isolated PostgreSQL 17 container.

- `go test -race ./...`: passed across the complete repository.
- `go test -race -tags=integration -run TestDashboardPostgres ./server/repository`: passed against PostgreSQL with an isolated schema and `TimeZone=Europe/Madrid`. Covers migrations, consistent measured aggregates across all three periods, legacy/modern telemetry, durable per-name limits, current phase progress, UTC receipt times, and worker lookup after migration.
- `go test -tags=integration -run TestServerWorkerIntegration -v ./integration`: passed against PostgreSQL, including job scheduling/assignment, synthetic pipeline events, successful finalization, final sizes, event history, and fresh-repository settings persistence.
- `golangci-lint run --timeout=5m ./...` using the repository's v2.11.4 image: zero issues.
- Server and worker binaries built successfully with `go build` and include the local dashboard assets.
- `npx -y @fission-ai/openspec@latest validate --all --strict`: passed before synchronization; repeated after archival.
- Chromium browser checks passed for login/logout and session revocation, charts without JavaScript/CSP errors, per-worker save/pause/isolation and pending acknowledgement, period changes, job filtering/search, empty states, failed refresh with retained data, and recovery. Checked overview and worker layouts at 320, 390, 768, 1024, and desktop widths with no page overflow. Preview data was a separate temporary fixture, not production data or an application fallback.

The unit suites additionally cover credential separation, wrong/expired/revoked sessions, Secure cookies for HTTPS deployment, CSRF protection, JSON/input validation, weighted compression and terminal-state boundaries, stale/abandoned progress, remote positive/zero/resume updates, synchronization failures, and serialization with local controls.

No new physical CPU/GPU encoding benchmark or production deployment was performed. Encoder profiles and FFmpeg command construction remain unchanged. Upgrade the server first, then workers, to activate runtime controls and telemetry. Existing naive historical timestamp rows are preserved; their original timezone cannot be inferred automatically.
