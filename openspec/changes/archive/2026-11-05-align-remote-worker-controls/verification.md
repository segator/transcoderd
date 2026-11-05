# Verification

Verified on 2026-11-05 with Go 1.26.10, local PostgreSQL 17 and Chromium against synthetic HTTP fixtures.

- `go test ./...`: all packages pass.
- `go test -race ./model ./server/repository ./server/scheduler ./server/web ./frontend/... ./worker/serverclient ./worker/worker`: all affected packages pass (server/web re-run after the final authentication test adjustment).
- Linux and Windows `golangci-lint run ./...`: zero issues.
- PostgreSQL `TestDashboardPostgres*` and control repository regressions: pass, including additive migration 010, retained pair/legacy resume limits and stale acknowledgements.
- Node frontend/PWA/resource/worker activity checks: 16 pass.
- Chromium dashboard regression script: pass, including Pause/Resume encodes, Stop/Resume queue, pending state, profile-aware card plots and exact 24h/7d/30d filtered worker counts, alongside previous playback/identity/pagination regressions.
- Strict OpenSpec validation and git diff whitespace checks pass.

## Behavioral review

A real owned child process stops progressing under remote RAM pause and resumes through the existing local process controller. Queue stop leaves that process running, holds queued starts, and queue resume restores previous capacity. Both directions preserve the independent RAM/queue and individual job holds. A failed acknowledgement is retried without replaying pause over a later local Resume. Commands require an online capable worker and reject maintenance; authenticated worker acknowledgements compare exact IDs. The resulting telemetry is published with the polling deadline before acknowledging success. Old clients do not receive commands.

Worker cards filter by selected reporting period inside the 30-day overview maximum and plot bounded profile-relevant CPU/RAM/GPU samples without persistent history. No worker listener is exposed and no remote computer shutdown was added.

## Delivery

The PR includes the earlier playback/library/identity fixes and this follow-up. GitHub CI and normal release publication are tracked after pushing; native Windows and full build/image/E2E gates are provided by that pipeline.
