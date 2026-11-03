# Verification

Verified 2026-11-03 against main 884c371 after integrating the current shared worker UI, Windows supervisor and queue/RAM pause behavior.

- `go test -race ./...`: passed. Focused regression reruns also passed after final scheduler/startup edits.
- Real local HTTP-to-server round trip in `worker/worker/dashboardSync_test.go`: atomic local CPU/GPU write -> server repository -> heartbeat -> authenticated fleet snapshot; remote fleet write -> polling -> local status. RAM hold survives pair changes. Queue stop blocks queued starts. Individual pause uses a real controlled child process and reaches fleet telemetry; general Resume preserves its individual hold. Failed persistence leaves the pair unchanged. Maintenance and finish holds remain closed during polling. Managed startup reads existing overrides without writing older defaults; no override keeps the local default without writes. Windows profile class/cap validation is checked through actual fleet routes.
- PostgreSQL 17 integration `TestDashboardPostgres`: passed, including migrations twice, durable limits/cache, rollback/commit failures, and persisted RAM/individual pause JSON read through a fresh repository instance.
- `golangci-lint run --timeout=5m` (v2.11.4 container): 0 issues.
- Linux server/worker build and Windows amd64 server/worker/service/updater cross-compilation: passed. Native Windows runtime was not executed in this Linux session.
- Shared worker activity JavaScript tests: 2 passed. JavaScript syntax and diff whitespace checks passed.
- Real Chromium against separate local/fleet pages: local CPU 2/GPU 3 appeared in fleet; fleet CPU 1/GPU 4 appeared locally; RAM hold persisted; queue stop appeared as Cola pausada; Resume restored the observed pair. Both pages fit 390/768/1280 px.
- Fleet pause rendering: individual RAM pause with preserved percentage and no running ETA, general RAM pause, held queued jobs, and fixed Windows profile inputs/cap rendered correctly without JavaScript errors.
- Dashboard regressions: auth/logout, charts, capacity saves, isolation, search/filter/period, empty/error/recovery and 320–1280 px responsive layouts passed. Media compatible playback/seek, original output playback, and explicit reviewed batch intake passed.
- Strict OpenSpec validation passed after syncing current capability paths; archive validation is recorded in the final change.

Browser fixtures use synthetic local media and in-memory repositories. No production deployment, production queue changes, GPU hardware validation or native Windows runtime validation was performed. The PR includes earlier fleet dashboard, independent encoder and media-library changes. Rollout requires the additive server migrations 007–009, then updated workers. RAM and individual holds remain runtime observations; configured desired limits persist by worker name.
