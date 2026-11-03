# Verification

Verified on 2026-11-03 with Go 1.26.10, PostgreSQL 17, local FFmpeg/ffprobe and headless Chromium through Playwright. No production jobs or media were modified. Read-only SSH inspection established the script criteria and existing server mounts.

## Automated code checks

- `go test -race ./...`: passed, including rooted-path containment, hidden/symlink rejection, cached metadata, strict preset boundaries/codec aliases, reviewed batch targets, stale/duplicate files, cancellation preserving committed work, operation bounds/expiry, real FFmpeg conversion and independent decoding, HTTP auth/CSRF/ranges, and serialized scheduler retries.
- `DASHBOARD_TEST_POSTGRES=... go test -race -tags=integration ./server/repository -run TestDashboardPostgres -count=1`: passed against a temporary PostgreSQL 17 container. Covers migrations twice, durable cache/fingerprint invalidation, job associations, callback rollback and deferred-constraint commit failure.
- `golangci-lint run --timeout=5m` using `golangci/golangci-lint:v2.11.4`: 0 issues.
- Server and worker `go build`: passed. JavaScript syntax checks and `git diff --check`: passed.
- `openspec validate --all --strict`: passed after synchronizing main specifications.

## Real browser checks

Synthetic 15-second H.264/AAC videos in isolated source/output folders, with queued and completed job fixtures; production builds contain no demo data.

- MKV compatible preview decoded at 640×360; full-duration seek to second 7 decoded successfully.
- Four subsequent rapid/repeated seek replacements decoded without HTTP 429. A discovered replacement race was corrected by coalescing seek updates and allowing a bounded 500 ms slot-release grace period without exceeding the process cap.
- Original MP4 output playback decoded directly from the read-only output root.
- Queued job source playback worked; a completed job with deleted source played its available output and labeled the missing source.
- Pause released the compatible stream after two seconds; resume decoded again; closing the dialog released the request/process.
- Folder navigation, preview, explicit eligible selection and successful visual batch submission passed. Output-root scans remained read-only; changing filters invalidated the prior preview.
- Existing login/logout, overview charts, independent worker CPU/GPU edits, pause, worker isolation, search/filtering, period selection, empty/error/recovery states remained functional.
- No JavaScript/CSP errors. All four views remained within the viewport at 320, 390, 768 and 1280 px widths; existing overview/worker checks also covered 1024 px.

## Boundaries

Compatible previews are deliberately converted playback aids, not original-quality evidence. Native original playback depends on browser codec/container support. Operation handles are temporary; jobs and probe cache persist. The default roots expose only the configured scheduler source directory; legacy `/target` requires the documented explicit `web.mediaRoots` setting. Production rollout remains separate from this implementation.
