# Tasks

## 1. Library data and scheduler integration

- [x] 1.1 Add rooted media models, metadata-cache migration/repository and batched job associations; verify cache invalidation, job roles and PostgreSQL persistence with tests.
- [x] 1.2 Implement source-only reviewed per-file intake, trusted probe reuse and intake serialization; verify duplicates, state-specific retries, fingerprints and successful-job targets.

## 2. Library and player backend

- [x] 2.1 Implement rooted browsing, asynchronous bounded scans/loads with cancellation, editable script-equivalent filters and durable probe caching; verify path containment, thresholds, sorting, limits and progress.
- [x] 2.2 Add authenticated original range streaming, compatible FFmpeg previews, seeking and process limits/cleanup; verify unauthorized access, real decoding, cancellation and unavailable files.
- [x] 2.3 Document roots, script provenance, filters, bitrate semantics, preview resource/quality boundaries, APIs and rollout; validate examples against handlers and config.

## 3. Dashboard experience

- [x] 3.1 Add library navigation, filters/presets, reviewed selection/batch progress and embedded job/file player; verify browser playback/seeking, visual loading, errors and responsive layouts.

## 4. Integrated verification

- [x] 4.1 Run Go/race and PostgreSQL tests, lint, server/worker builds, real browser playback and strict OpenSpec validation; compare delivered behavior with every delta scenario.

## Workflow follow-up

- Synchronize main specs and archive the completed change after verification.
