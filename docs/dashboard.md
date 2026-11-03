# TranscoderD operations dashboard

The server embeds its dashboard and all assets. Open `http://<server>:8080/dashboard/` (the server root redirects there) and log in with the dashboard token. The initial default is `encoding123`, as requested; configure it independently from the worker API token:

```yaml
web:
  port: 8080
  domain: https://transcoder.example.com
  token: your-worker-api-token
  dashboardToken: your-dashboard-token
```

Or pass `--web.dashboardToken encoding123` to the server. The server exposes no additional management port, requires no frontend build, and loads no remote scripts/fonts. The dashboard has a separate authentication boundary for a future OIDC implementation. Worker API authentication continues using `web.token`.

Sessions last 12 hours and use an opaque HttpOnly SameSite=Strict cookie. Logout revokes the session; server restart revokes all sessions. When publishing through an HTTPS proxy, set `web.domain` to the HTTPS public origin so cookies have Secure set. Keep the request Host header as the public host and forward `/dashboard/` on the same origin. The dashboard does not require an external Sites deployment. The static token stays out of URLs, page source, browser storage, and request logs. Dashboard mutations require `X-Dashboard-Request: 1` and reject another Origin.

## Independent CPU/GPU limits inside each worker

1. Upgrade the server first; it applies migrations 007 (telemetry) and 008 (independent limits) automatically.
2. Upgrade workers to enable both encoder classes, runtime polling, and version-2 capacity telemetry.
3. Open **Workers**. Each named worker has separate **CPU** and **GPU** active counters, encoder names, and concurrency inputs.
4. Enter both non-negative limits and choose **Guardar límites**. For example CPU `2` plus GPU `3` enables two simultaneous software encodes and three NVENC encodes in the same process. One atomic database write saves the pair under that worker's stable name; other workers remain unchanged.
5. Set either field to zero to disable that class for new unassigned encodes. **Pausar ambos** saves `0 / 0` and stops new admission. Accepted work continues; save positive values to resume when the configured schedule permits.

The worker synchronizes settings before each normal admission evaluation, approximately every five seconds, even while paused. Its following heartbeat reports observed per-class limits and counts. Cards show pending acknowledgement until desired and observed pairs match. Offline workers retain settings for reconnect/restart. During a settings-read failure, the coordinator skips admission while accepted jobs continue. The updater's temporary drain never persists zero and does not enter another synchronization cycle.

Admission is bounded by `CPU + GPU + 2` accepted jobs: the extra two overlap source preparation and verification/upload. Each encoder class separately enforces its own concurrency. Lowering a positive limit lets its running encodes finish before starting another. Disabling one class sends new unassigned jobs to the other. Jobs whose encoding profile was already pinned before restart may resume on their disabled class using retained drain capacity (minimum one if the old target is unavailable). With both disabled, accepted queued jobs drain using prior positive per-class capacity. Running/draining counts can temporarily exceed the new desired value; the UI labels this state. Slot counts do not measure hardware utilization.

The startup `worker.ffmpegConfig` remains the preferred encoder/profile. Its class uses that exact recipe. The other class defaults to approved CPU SVT-AV1 preset 4/CRF25 or GPU NVENC p7/CQ38. Audio/thread settings are shared initially; encoder quality flags and hardware-specific `extraArgs` never leak into the other class. Optional YAML `worker.cpuFFmpegConfig` and `worker.gpuFFmpegConfig` specify separate profiles (see the configuration reference). A selected profile is copied and persisted in the job context before encoding, so restart and later configuration changes preserve the job's chosen backend and parameters. When only one class is enabled, the worker pins that class at admission (including an in-flight assignment arriving after a pause), so a CPU-only accepted job cannot switch to GPU after restart.

A CPU-only host should keep GPU `0`. Enabling GPU slots requires the documented compatible NVIDIA GPU, driver, and container runtime; encoding does not silently fall back to CPU. No GPU command runs when GPU slots are disabled unless previously accepted pinned GPU work is being restored.

Existing rows without a pair keep their single-limit startup behavior. Positive local `control set-max-jobs N` replaces a pair with the startup selected encoder at `N`; zero pauses admission and allows accepted work to drain. Legacy workers still accept presence, but lack independent execution. When such a worker reads a stored pair without declaring support, the server returns aggregate zero so it safely drains until upgraded. The dashboard labels the required upgrade. Keep `worker.name` stable across container replacement. Drain all accepted work before downgrading a dual-capable worker: older binaries do not honor its saved per-job profile snapshots.

## Media playback and visual intake

The **Biblioteca** view browses source folders and optional read-only output roots, filters and previews candidates, loads reviewed selections with progress/cancellation, and plays available files directly or through a compatible preview. **Ver** in job tables opens available source/completed output assets. See [Media library and visual job loading](media-library.md) for roots, presets matching the Unraid scripts, resource limits and the API. Migration 009 adds the fingerprinted metadata cache.

## What the charts measure

- **Recovered space:** all-time paired positive source/output sizes for jobs whose current outcome is `Job/completed`. The saving is the signed byte difference; reduction percentage is weighted by total source bytes. A completed FFmpeg phase is still an active job. Missing or zero output sizes do not produce invented savings.
- **Completion/failure history:** current terminal job outcomes by their completion timestamp, over a rolling 24-hour, 7-day, or 30-day period. Retries that are currently queued/active are represented by their current state, not double-counted as completed outcomes.
- **Success rate:** completed / (completed + failed) in the selected period; canceled jobs are excluded. An empty denominator shows unavailable data.
- **Average processing time:** last Job/started to Job/completed for current completed jobs in the selected period; queue wait is excluded. Missing start timestamps are excluded from the mean.
- **Queue distribution:** all current jobs, separated into queued, active, completed, failed, and canceled. Intermediate phase completion does not make a job terminal.
- **Savings over time and codec compression:** finalized paired sizes in the selected period. Output video codec comes from its probe; legacy records without one are labeled unavailable. Compression alone does not establish visual quality.
- **Worker results:** completions, failures, and bytes recovered in the selected period; observed CPU/GPU counters and configured limits come from heartbeats. Online means a heartbeat was received within 30 seconds. Offline counters are explicitly last-observed values.
- **Job progress:** current assignment and started phase only; its ETA covers that phase. Progress older than one minute is marked stale. Multiple OCR subtasks use mean percentage and the maximum reported ETA; this is not a whole-job or whole-fleet completion forecast.

Charts have descriptive accessible names and hover values. The browser refreshes every five seconds while visible; worker progress is normally published approximately every 15 seconds. The period selector controls period metrics, while global queue/storage totals stay accumulated. Queries aggregate in PostgreSQL under a consistent read snapshot. The table shows at most 100 jobs, prioritizing active work and queue entries; filters/search apply to that displayed sample. Activity is capped at 30 events, with five visible on the overview. Empty and disconnected states remain explicit, without demo data in production.

New monitoring timestamps are recorded in UTC regardless of server/database timezone. Existing naive historical rows are preserved. If an older deployment wrote local-time timestamps, its historic bucket boundaries may be offset until an operator explicitly corrects those rows using the known original timezone; no automatic conversion guesses that timezone.

## Verification

```bash
go test -race ./...
DASHBOARD_TEST_POSTGRES='host=127.0.0.1 port=5432 user=postgres password=postgres dbname=postgres sslmode=disable' \
  go test -tags=integration -run TestDashboardPostgres -v ./server/repository
npx -y @fission-ai/openspec@latest validate --all --strict
```

The PostgreSQL test creates and removes an isolated schema; its database user needs schema creation permission. Browser verification covers login/logout, no credential exposure, capacity save/pause and worker isolation, period switching, job search/filtering, and mobile overflow. Screenshot previews use separate local test fixtures, never production dashboard data.
