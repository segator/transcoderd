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
| `self-update` | `update/update.go`, `update/managed/`, `cmd/windows-service/updates_windows.go`, `server/web/web.go`, `worker/worker/coordinator.go` |
| `operations-dashboard` | `frontend/`, `server/web/dashboard*`, `server/repository/dashboard.go`, `model/dashboard.go`, `worker/worker/coordinator.go` |

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

## Resolved implementation discrepancies

- Before `pause-queued-encodes`, the implemented two-job prefetch buffer exceeded the unsynchronized main admission contract; zero admission continued prefetched encodes, and Windows startup could replace local profile limits with stale server values. The change reduces the buffer to one, separates user pause from maintenance drain and gives service-owned profiles authoritative startup limits.

- Native Windows CPU CI exposed a download close failure: syncing the read-only source invokes Windows FlushFileBuffers without write access and prevents checksum publication. Download streams now close without a write flush, then publish their checksum; upload streams retain their sync. A read-only download regression and the complete native E2E cover this fix.

- PR 187 initially replaced the Linux builder's FFmpeg hash with the official release tarball hash, incorrectly treating the upstream pin as stale. CI demonstrated that Linux downloads the GitHub n9.0.2 tag archive, whose verified SHA-256 is `6e374ed621e48faa40639307dff48ba6fe574a509977956d2cce9669b7cc27e9`. The override was removed. Windows retains the separate official tar.gz (`84960df915059e8754fef2cd7c9afeb614062b1b5458ec471eecee619ee04e98`), verified with FFmpeg release key `FCF986EA15E6E293A5644F10B4322F04D67658D8`. Both recipes retain source version 9.0.2 and their intended archive/hash pair.

## Verification boundary

Dashboard investigation identified two pre-existing contract discrepancies: main job-dispatch specs still described the prefetch-free admission ceiling and local-only persisted controls even though completed active deltas `keep-encoder-prefetched` and `persist-worker-job-limit` describe shipped code. Those deltas remain separate changes; dashboard capacity displays follow the implemented encode-target-plus-two ceiling. The dashboard change explicitly supersedes the local-only operator restriction while keeping the worker listener local. It also fixes the worker lookup's obsolete `queue_name` scan, which disagreed with migration 003 and would break reads after the additive telemetry migration; PostgreSQL regression coverage verifies that lookup.

`openspec validate --all --strict` checks document structure. Go unit tests cover parts of scheduling, media inspection, context persistence, and steps. PostgreSQL integration tests and Docker end-to-end tests use the `integration` build tag. Structural validation does not prove behavior; each future code change needs tests for the scenarios it affects.


### Media library and reviewed intake (2026-11-03)

`server/library/` implements rooted discovery, metadata caching, bounded scan/load operations and compatible/original playback; `server/web/media.go` exposes authenticated dashboard routes. Migration 009 persists fingerprinted probes. The source root is queueable and optional `web.mediaRoots` are read-only. See `docs/media-library.md` for inspected Unraid script provenance and mount examples. Legacy Plex-only metadata type selection is intentionally replaced by filesystem discovery within operator-selected folders.

Investigation found `SQLRepository.WithTransaction` previously committed callback failures and could hide commit errors because its deferred closure observed an unchanged local error. The media-library change fixes rollback/commit propagation and includes real PostgreSQL callback-rollback and deferred-constraint commit-failure tests. Per-file intake is serialized across dashboard and legacy requests to prevent duplicate creation within one scheduler process; trusted reviewed probes avoid a second unrooted probe. Existing legacy recursive job intake has its own extension/filter semantics; the new rooted library does not silently rewrite those contracts.
- Complete Windows upgrades previously assumed the configured workspace was outside the installation tree, although desktop configuration accepted nested paths. The mandatory updater change rejects those paths (including existing junction aliases) during configuration and bootstrap before Setup can replace files; existing affected installations must choose an external workspace before bootstrap.

- Consecutive managed Windows updates exposed stale completed-journal recovery clearing a newer pending target after downloading it, falsely reporting success and reopening admission. The coordinator now requires matching pending release and signed candidate identity before clearing a completed barrier; regression coverage retains newer targets through drain and installation.

- Manual Windows Setup previously invoked the installed helper and aborted at the first busy CPU profile, potentially leaving GPU admission open. Setup now uses its extracted helper to hold both profiles and wait for accepted work before replacement. Immediate service preparation also drains/checks both profiles and rejects missing configured workers.


### Worker UI synchronization and current main integration (2026-11-03)

The dashboard branch was integrated with main through 884c371, including the shared worker UI, Windows service and queue/RAM controls. Main changed the pipeline buffer from two to one and operator zero from draining all queued encodes to holding queued starts. The synchronization change reconciles those observable contracts explicitly; archived earlier evidence remains historical. It also replaces managed startup's unconditional server overwrite with existing-override restoration, with read-only fallback to local defaults for an empty store. Local pair writes persist first, and worker polling serializes with local controls. RAM toggles previously reset independent limits through the single-limit setter; they now preserve the pair and individual holds. Fleet telemetry records queue/RAM/job/maintenance observations separately from desired settings. `worker/worker/dashboardSync_test.go` exercises real local HTTP -> worker API -> repository -> heartbeat -> fleet data and the reverse direction.


### Installable dashboard PWA (2026-11-04)

`server/web/dashboardPWA.go` versions the scoped worker and allowlisted embedded public assets by content; the dashboard/login metadata and branded icons support installation. Offline navigation receives only the generic reconnect page. Neither authenticated HTML nor APIs/media/credentials enter CacheStorage, and background mutations are never replayed. `dashboardPWA_test.go` and Node lifecycle tests cover HTTP/auth and cache boundaries; real Chromium verifies installability, offline/reconnect/logout and update behavior. Investigation also found that the mobile sidebar hid the only logout button; the PWA change adds a mobile topbar logout action. Runtime fallbacks use non-creating cache lookups so an old in-flight worker cannot resurrect its deleted cache after activation.

### Independent fleet frontend (2026-11-04)

The operator changed the deployment requirement after the embedded UI implementation: the frontend host has API network access only. This is an intentional superseding architecture change, not an undocumented regression. `frontend/` now owns fleet assets, PWA and a restricted streaming proxy; `server/web/` owns dashboard/session/media APIs without frontend assets. Earlier embedded-server verification remains historical. The worker-local UI stays separate from fleet hosting. Frontend configuration and build/image targets are documented in the configuration and dashboard references.

Local image verification also found the previous Docker loader assumed OCI archives create a role:latest tag, although docker load returned an untagged image ID. Application image exports now use the pinned Dagger SDK host-image export with each requested tag, retaining the OCI archive fallback. This restores the documented usable local image tags for the new frontend and existing roles.

### Worker hardware monitoring (2026-11-04)

`worker/resources/` samples OS-visible CPU/RAM and read-only NVIDIA metrics in the worker lifecycle. Optional model resources flow through existing heartbeat/runtime JSON into the independent frontend. Overview navigation selects worker details with bounded in-memory resource plots. Slot counts remain capacity, not utilization; unsupported sensors, CPU warm-up and stale samples stay explicit. No monitoring schema migration or frontend access to host counters is introduced.


### Frontend OIDC and Kubernetes (2026-11-04)

The frontend now owns optional OIDC browser login while API session validation/revocation remains server-owned. Token mode stays the compatibility default. `frontend/web/oidc.go` validates discovery, code/PKCE, state/nonce and signed identity/group claims, then encrypts the API session for interchangeable frontend replicas. OIDC mode refuses token/legacy-cookie fallback. The historical API-only frontend constraint now also permits identity-provider access and runtime auth secrets; media/database access remains unnecessary. Session/key/provider/logout limitations and k3s/Traefik examples are documented in `docs/frontend-authentication.md` and `deploy/kubernetes/frontend/`.


### SVT CRF default and saved profiles (2026-11-04)

Before `lower-svt-default-crf-26`, profile pinning copied omitted SVT CRF as zero, so persisted jobs could resolve against a later binary default. This disagreed with the immutable per-job profile contract. The change stores the resolved CRF for new SVT profiles and restores legacy zero snapshots as CRF25, the default present when profile persistence was introduced (c98e80d). Explicit saved overrides remain unchanged. Disk round-trip regression tests cover new CRF26 work, old implicit CRF25 work and explicit qualities across later startup configuration changes. The selected CRF26 default is based on the recorded short-scene size/quality trade-off, not a new quality guarantee.

### Cross-platform CI and cache boundaries (2026-11-04)

The previous operational references described application builds as reusing published builder tags, although the code evaluates source build graphs on the connected Dagger Engine. That documentation discrepancy is corrected; runs 41876078723 and 41892573798 connected to the same Kubernetes Engine and demonstrated cached media dependencies. The tooling-only `normalize-cross-platform-ci` change consolidates lint/Linux/Windows cross-builds in `ci`, bounds Windows compiler parallelism to an aggregate budget of 16, isolates dependency cache keys and avoids ordinary CI image archive exports. Moving Windows artifacts into the same workspace also requires an explicit Linux release-asset allowlist to preserve native Windows publication gates. Installer zlib level 3 retains the complete payload and default deduplication; same-payload measurements and SHA-256 verification are recorded in `docs/ci-pipeline.md`. Existing distribution/runtime contracts are unchanged.

### Frontend ARM64 and ordinary CI consolidation (2026-11-05)

The original frontend image recipe explicitly compiled amd64 and its Kubernetes example selected only amd64 nodes; this was a documented implementation limit. `frontend-arm64-and-ci-jobs` adds Linux amd64/arm64 image variants and one shared OCI index, with static ELF, version, CA, nonroot health and archive checks. Local foreign-CPU execution uses emulation, not a native ARM hardware/performance claim. The Kubernetes example admits the two supported Linux architectures. Ordinary CI now has one Linux build/check job and one native Windows job; current source/web tooling is saved outside tagged checkouts, frontend Windows units join the cross-built suites and old tags retain their frontend Go-test fallback. Artifact debugging is manual-only with read-only credentials, preserving its distinct validation/publication boundary.
