# Verification

Verified on 2026-11-05 with Go 1.26.10, PostgreSQL 17 in an isolated local container and Chromium against synthetic HTTP fixtures.

- `go test ./...`: all packages pass.
- `go test -race ./model ./server/library ./server/repository ./server/web ./frontend/web ./server/scheduler`: all affected packages pass.
- `DASHBOARD_TEST_POSTGRES=... go test -tags=integration -run TestDashboardPostgres ./server/repository`: existing dashboard SQL coverage plus filtered page starvation/search/navigation and exact 30-day membership pass.
- `node --test frontend/web/*test.cjs`: 13 tests pass, including fixed profile hardware scope and private PWA caching boundaries.
- `python frontend/web/dashboard_browser_test.py` using an existing Chromium executable: identity, completed rows beyond 100 queued jobs, search and pagination, original-file playback/native seeking, single timeline, close cleanup, atomic CPU-only save, profile-specific hardware/controls, offline cards, worker pagination, scanner retry provenance and mobile identity/layout pass.
- `openspec validate --all --strict`: all specs and active changes pass before synchronization; repeated after synchronization/archive.
- `git diff --check`: clean.

## Contract review

Queued jobs expose only source assets; active/phase-completed/failed/canceled jobs reject job-assets playback; completed jobs expose only outputs, including after source removal and without source substitution when output is missing. Direct library playback remains independent. Browser code never requests a compatible preview and unsupported originals display a format/availability error.

Fleet membership is independent of the statistics period and excludes unknown/older presence without deleting records. Online/active order and boundary inclusion are covered by model and SQL tests. Fixed Windows profiles submit the unsupported class as zero and show relevant hardware; independent workers retain both classes. Offline controls disappear even when a draft was focused at disconnect.

Display claims originate in verified signed ID tokens, survive replica changes inside encrypted cookies, and are JSON-size bounded to fit browser cookie limits. Anonymous/revoked sessions cannot fetch identity. Old sessions require one new sign-in because prior code discarded identity claims.

## Limits

No production deployment or external configuration changes were performed. Browser tests use synthetic data and a generated H.264 MP4; compatibility of original MKV/AV1/audio depends on the operator's browser. Native Windows worker/service execution is unchanged; the existing managed-profile telemetry drives presentation.
