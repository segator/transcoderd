# OpenSpec baseline

These main specs inventory the behavior of the reviewed repository. They were added directly under `openspec/specs/` because this is the initial description of the existing system, not a proposal to ship new functionality. Future behavior changes belong in `openspec/changes/` as delta specs and must be synchronized when archived.

The specs state observable behavior and the configuration values that affect it. [`CONFIGURATION.md`](CONFIGURATION.md) gives the full current settings reference and example worker YAML. [`PROTOCOL.md`](PROTOCOL.md) documents HTTP routes and wire payloads. Exact command construction and internal control flow remain in the source code.

| Capability | Primary evidence |
| --- | --- |
| `configuration-and-runtime` | `cmd/`, `server/main.go`, `worker/main.go`, `server/config/`, `worker/config/` |
| `distribution` | `Makefile`, `Dockerfile`, `version/`, `.github/workflows/` |
| `job-intake` | `server/web/web.go`, `server/scheduler/scheduler.go`, `model/model.go` |
| `job-dispatch` | `server/scheduler/scheduler.go`, `worker/worker/coordinator.go`, `worker/worker/executor.go` |
| `events-and-progress` | `server/scheduler/scheduler.go`, `server/repository/repository.go`, `worker/worker/progress.go` |
| `media-transfer` | `server/web/web.go`, `server/scheduler/streams.go`, `worker/step/downloadStep.go`, `worker/step/uploadStep.go` |
| `media-encoding` | `worker/ffmpeg/ffprobe.go`, `worker/step/ffmpegStep.go`, `worker/step/ffmpegVerifyStep.go` |
| `subtitles` | `worker/ffmpeg/ffprobe.go`, `worker/step/MKVExtractStep.go`, `worker/step/PGStoSrtStep.go` |
| `persistence` | `server/repository/repository.go`, `server/repository/resources/database/` |
| `maintenance-and-recovery` | `server/scheduler/scheduler.go`, `worker/job/job.go`, `worker/worker/executor.go` |
| `self-update` | `update/update.go`, `server/web/web.go`, `worker/worker/coordinator.go` |
| `operations-dashboard` | `server/web/dashboard*`, `server/repository/dashboard.go`, `model/dashboard.go`, `worker/worker/coordinator.go` |

## Known implementation gaps

These observations are not normative requirements. They need separate fixes or explicit product decisions, each with a regression test:

- `CancelJob` appends `Job/canceled` for assigned or started jobs, then returns an unknown-state error. The API can answer HTTP 500 despite recording cancellation. Queued jobs are not canceled through that route.
- Download, checksum, and upload routes do not use the bearer middleware used by the four control routes. Download and upload check `workerName` against the assignment; checksum only looks up the UUID and a published checksum.
- An invalid upload returns HTTP 400, but deferred stream close still renames the `.upload` file to the destination. Cleanup can remove the previous destination. The old file is not guaranteed to be preserved.
- The registered `worker.threads` flag is never wired to `worker.ffmpegConfig.threads`, which supplies FFmpeg's `-threads` option. Without the nested setting, the current invocation uses `-threads 0`.
- The intake response includes `skipped`, but the current scheduler classifies ineligible files under `failed`; this path does not produce `ErrorFileSkipped`.
- Several entries in the accepted-extension list include trailing spaces, so ordinary `.mov`, `.ogx`, `.asf`, and `.vob` files fail the extension check; matching is also case-sensitive.
- Job intake joins the submitted `sourcePath` with the media root but does not reject traversal segments or verify that the resolved path remains inside the root.
- Viper enables a `TR_` environment prefix, but does not configure a nested-key replacer. The `envconfig` struct tags are not read by the Viper setup, so nested environment-variable spellings are not a verified interface.
- Historical notification/presence columns are naive SQL timestamps. Before the dashboard change, notification and presence writes used server local time while progress used database session time. The dashboard change normalizes new monitoring writes and timeout comparisons to UTC, but cannot infer or rewrite the timezone of old rows. Historical dashboards on previously non-UTC deployments may require an explicit operator-led timestamp correction.

## Verification boundary

Dashboard investigation identified two pre-existing contract discrepancies: main job-dispatch specs still described the prefetch-free admission ceiling and local-only persisted controls even though completed active deltas `keep-encoder-prefetched` and `persist-worker-job-limit` describe shipped code. Those deltas remain separate changes; dashboard capacity displays follow the implemented encode-target-plus-two ceiling. The dashboard change explicitly supersedes the local-only operator restriction while keeping the worker listener local. It also fixes the worker lookup's obsolete `queue_name` scan, which disagreed with migration 003 and would break reads after the additive telemetry migration; PostgreSQL regression coverage verifies that lookup.

`openspec validate --all --strict` checks document structure. Go unit tests cover parts of scheduling, media inspection, context persistence, and steps. PostgreSQL integration tests and Docker end-to-end tests use the `integration` build tag. Structural validation does not prove behavior; each future code change needs tests for the scenarios it affects.


### Media library and reviewed intake (2026-11-03)

`server/library/` implements rooted discovery, metadata caching, bounded scan/load operations and compatible/original playback; `server/web/media.go` exposes authenticated dashboard routes. Migration 009 persists fingerprinted probes. The source root is queueable and optional `web.mediaRoots` are read-only. See `docs/media-library.md` for inspected Unraid script provenance and mount examples. Legacy Plex-only metadata type selection is intentionally replaced by filesystem discovery within operator-selected folders.

Investigation found `SQLRepository.WithTransaction` previously committed callback failures and could hide commit errors because its deferred closure observed an unchanged local error. The media-library change fixes rollback/commit propagation and includes real PostgreSQL callback-rollback and deferred-constraint commit-failure tests. Per-file intake is serialized across dashboard and legacy requests to prevent duplicate creation within one scheduler process; trusted reviewed probes avoid a second unrooted probe. Existing legacy recursive job intake has its own extension/filter semantics; the new rooted library does not silently rewrite those contracts.
