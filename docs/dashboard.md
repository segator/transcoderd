# TranscoderD operations dashboard

The fleet dashboard runs in the separate `transcoderd-frontend` process/container. Open `http://<frontend>:8081/dashboard/` and sign in with a single-user token (default mode) or configured OIDC provider. TranscoderD serves only the API; it no longer serves dashboard HTML, scripts, icons or PWA assets. The initial default is `encoding123`, as requested; configure it independently from the worker API token:

```yaml
web:
  port: 8080
  domain: http://transcoder-api.internal:8080
  token: your-worker-api-token
  dashboardToken: your-dashboard-token
```

Or pass `--web.dashboardToken encoding123` to the server. The server exposes no additional management port, requires no frontend build, and loads no remote scripts/fonts. The frontend can alternatively use OIDC with Authentik; see [authentication and k3s deployment](frontend-authentication.md). Worker API authentication continues using `web.token`.

In token mode, sessions last 12 hours and use an opaque HttpOnly SameSite=Strict cookie. OIDC uses a separate encrypted frontend cookie and is described in the authentication guide. Logout revokes the session; server restart revokes all sessions. When publishing the frontend through an HTTPS proxy, set `frontend.publicURL` to the HTTPS browser origin so cookies have Secure set. Route `/dashboard/` to the frontend, not to the API server. Keep `web.domain` pointing to the API origin used by workers. The dashboard does not require an external Sites deployment. The static token stays out of URLs, page source, browser storage, and request logs. Dashboard mutations require `X-Dashboard-Request: 1` and reject another Origin.


## Deploy on a separate host

```bash
mage build:frontend
./dist/transcoderd-frontend \
  --frontend.listen=:8081 \
  --frontend.apiURL=http://transcoder-api.internal:8080 \
  --frontend.publicURL=https://transcoder.example.com
```

Point the public HTTPS reverse proxy at the frontend port. Set `frontend.publicURL` to the actual browser origin when TLS terminates there; client-supplied forwarded host/scheme headers are ignored. API/public URL settings accept HTTP(S) origins only, without credentials, non-root paths, query or fragments. The default API origin is `http://localhost:8080`; the default listener is `:8081`; an empty public URL derives the direct HTTP/TLS request origin. YAML uses `frontend.listen`, `frontend.apiURL` and `frontend.publicURL`; explicit environment names are `TR_FRONTEND_LISTEN`, `TR_FRONTEND_API_URL` and `TR_FRONTEND_PUBLIC_URL`. `--config /etc/transcoderd/frontend.yaml` selects a file, otherwise the usual config.yaml search paths apply. CLI overrides environment and YAML.

The separately published image is `ghcr.io/segator/transcoderd:frontend-<version>`
with Linux amd64 and arm64 variants under the same tag. Docker/Kubernetes select
the matching variant; use a released multi-platform tag or index digest on ARM64,
since older tags are unchanged. `mage docker:frontend` exports both variants in
`dist/frontend-image.tar` and loads the host-compatible local image.
`mage test:frontendImages` checks both static ELF architectures, version output,
nonroot HTTP health and the OCI index. Foreign-architecture runtime checks use
emulation on an amd64 Engine; they are not a native ARM performance benchmark.
The standalone `mage build:frontend` executable keeps its existing host-target
behavior. Server and media-worker image architectures are unchanged.

Example on either supported frontend host:

```bash
docker run -d --name transcoderd-frontend --read-only --cap-drop=ALL \
  --security-opt=no-new-privileges -p 8081:8081 \
  -e TR_FRONTEND_API_URL=http://transcoder-api.internal:8080 \
  -e TR_FRONTEND_PUBLIC_URL=https://transcoder.example.com \
  ghcr.io/segator/transcoderd:frontend-<version>
```

No media volumes, PostgreSQL access, FFmpeg or worker token are configured on the frontend host. Token mode needs no frontend credential. OIDC mode additionally requires a private API dashboard credential and OIDC/session secrets as described in the [authentication guide](frontend-authentication.md). Network access to the API is required, plus the identity provider in OIDC mode; API HTTPS uses normal certificate verification. Use a protected network or HTTPS between hosts. The browser sends login, metadata, operations and videos to its frontend origin; the frontend streams only allowlisted dashboard API routes to the configured backend. Worker `/api/v1` routes cannot be reached through it. The API validates dashboard credentials and owns expiring sessions; the frontend forwards only the dashboard session cookie and preserves HttpOnly/SameSite restrictions. Logout and API restart revoke access. API unavailability returns an explicit error and never substitutes cached operational data.

Media discovery, filters, probes, reviewed intake, original byte ranges and compatible H.264/AAC conversion all run on the API host. Video bytes flow through the frontend without complete-file buffering or local storage. Closing playback cancels the backend request and preview. The frontend image runs as a nonroot user and contains no media toolchain. Its `/healthz` endpoint reports frontend liveness, independently of backend availability.

For migration, assign separate API and frontend hostnames, upgrade the API and start the frontend. Keep workers pointed at the API origin. If an older deployment used one hostname for both UI and worker requests, update worker `web.domain` to the API hostname before moving the old dashboard hostname to the frontend. Existing server-side UI URLs now return 404; sign in again on the frontend origin. Local worker/Windows tray dashboards remain worker-local and are independent of this fleet frontend.
## Independent CPU/GPU limits inside each worker

1. Upgrade the server first; it applies migrations 007 (telemetry) and 008 (independent limits) automatically.
2. Upgrade workers to enable both encoder classes, runtime polling, and version-2 capacity telemetry.
3. Open **Workers**. Independent named workers have separate **CPU** and **GPU** encoder counters and limits. Fixed Windows profiles expose only their supported CPU or GPU class.
4. Enter both non-negative limits and choose **Save limits**. For example CPU `2` plus GPU `3` enables two simultaneous software encodes and three NVENC encodes in the same process. One atomic database write saves the pair under that worker's stable name; other workers remain unchanged.
5. Set either field to zero to disable that class for new unassigned encodes. **Stop after current** holds admission and queued starts while accepted encodes finish. **Resume queue** restores positive limits. **Pause in RAM** / **Resume encodes** independently suspend/resume owned encodes, retaining their memory and existing queue/individual holds. Commands show pending until acknowledged; older workers must update to enable remote controls.

The worker synchronizes settings before each normal admission evaluation, approximately every five seconds, even while paused. Its following heartbeat reports observed per-class limits and counts. Cards show pending acknowledgement until desired and observed pairs match. Offline workers retain settings for reconnect/restart. During a settings-read failure, the coordinator skips admission while accepted jobs continue. The updater's temporary drain never persists zero and does not enter another synchronization cycle.

Admission is bounded by `CPU + GPU + 1` accepted jobs: the extra job overlaps preparation or verification/upload. Each encoder class separately enforces its own concurrency. Lowering a positive limit lets its running encodes finish before starting another. Disabling one class sends new unassigned jobs to the other. Jobs whose encoding profile was already pinned before restart may resume on their disabled class using retained drain capacity (minimum one if the old target is unavailable). With both disabled, pending encodes wait; running encodes and downstream work finish. Maintenance draining explicitly releases queue/RAM/individual holds without persisting a new preference. Running/draining counts can temporarily exceed the new desired value; the UI labels this state. Slot counts do not measure hardware utilization.

The startup `worker.ffmpegConfig` remains the preferred encoder/profile. Its class uses that exact recipe. The other class defaults to approved CPU SVT-AV1 preset 4/CRF26 or GPU NVENC p7/CQ38. Audio/thread settings are shared initially; encoder quality flags and hardware-specific `extraArgs` never leak into the other class. Optional YAML `worker.cpuFFmpegConfig` and `worker.gpuFFmpegConfig` specify separate profiles (see the configuration reference). A selected profile is copied and persisted in the job context before encoding, so restart and later configuration changes preserve the job's chosen backend and parameters. When only one class is enabled, the worker pins that class at admission (including an in-flight assignment arriving after a pause), so a CPU-only accepted job cannot switch to GPU after restart.

A CPU-only host should keep GPU `0`. Enabling GPU slots requires the documented compatible NVIDIA GPU, driver, and container runtime; encoding does not silently fall back to CPU. No GPU command runs when GPU slots are disabled unless previously accepted pinned GPU work is being restored.

Existing rows without a pair keep their single-limit startup behavior. Positive local `control set-max-jobs N` replaces a pair with the startup selected encoder at `N`; zero pauses admission and queued encode starts while running/downstream work finishes. Legacy workers still accept presence, but lack independent execution. When such a worker reads a stored pair without declaring support, the server returns aggregate zero so it safely pauses until upgraded. The dashboard labels the required upgrade. Keep `worker.name` stable across container replacement. Drain all accepted work before downgrading a dual-capable worker: older binaries do not honor its saved per-job profile snapshots.

## Media playback and visual intake

The **Library** view browses source folders and optional read-only output roots, filters and previews candidates, loads reviewed selections with progress/cancellation, and plays original file bytes with native browser controls and one timeline. **View source** is offered for queued jobs; **View output** is offered only for completed jobs. Active/failed/canceled jobs have no playback action, and missing outputs never fall back to the source. See [Media library and visual job loading](media-library.md) for roots, presets matching the Unraid scripts, resource limits and the API. Migration 009 adds the fingerprinted metadata cache.

## Worker versions

Worker cards, overview summaries and selected resource details show the running binary version reported by that worker. Offline cards retain it as **last reported**. Missing fields from older workers show **Version unavailable**; protocol capabilities and the API version do not identify the worker release. Normal heartbeat/polling refreshes update the value. Upgrade the API and frontend, then workers, to populate this additive telemetry; no database migration or external version lookup is required.

## CPU/GPU resource details

Overview worker rows show measured CPU, GPU and NVENC utilization and link to that worker in the Workers view. Online worker cards list current tasks with names, phase progress and pause state from worker heartbeats, independent of the overview job cap. Capability-version-2 workers expose **Pause** / **Resume** for each owned controllable encode; older workers show an update hint. Task resume clears only its individual hold and can remain suspended by the general RAM hold. Both server and worker reject stale/uncontrollable targets. Commands are acknowledged through the existing per-worker slot. Online worker cards include compact CPU/RAM/GPU/NVENC plots from up to 120 samples/ten minutes received in the current tab; fixed profiles show only their relevant encoder hardware plus shared RAM. Stale or unavailable samples remain explicit. No history is persisted in the browser. Details include recent CPU/RAM and per-device GPU/encoder graphs, VRAM, temperature, power and freshness, with the existing controls underneath. Host/device measurements include other processes and are distinct from encode slots. See [worker resources](worker-resources.md) for platform scope, missing/stale readings and collection requirements.

## What the charts measure

- **Space saved:** separate selected-period and all-time paired positive source/output sizes for jobs whose current outcome is `Job/completed`. The saving is the signed byte difference; reduction percentage is weighted by total source bytes. A completed FFmpeg phase is still an active job. Missing or zero output sizes do not produce invented savings.
- **Completion/failure history:** current terminal job outcomes by their completion timestamp, over a rolling 24-hour, 7-day, 30-day or custom date period. Custom dates use the browser timezone and include the chosen last day; the API receives start-inclusive/end-exclusive UTC timestamps, including daylight-saving transitions. Ranges are limited to 3660 days. A frontend running against an older API refuses custom results without matching returned boundaries and keeps the previous results labeled with their original period. Retries that are currently queued/active are represented by their current state, not double-counted as completed outcomes.
- **Success rate:** completed / (completed + failed) in the selected period; canceled jobs are excluded. An empty denominator shows unavailable data.
- **Average processing time:** last Job/started to Job/completed for current completed jobs in the selected period; queue wait is excluded. Missing start timestamps are excluded from the mean.
- **Queue distribution:** all current jobs, separated into queued, active, completed, failed, and canceled. Intermediate phase completion does not make a job terminal.
- **Savings over time and codec compression:** finalized paired sizes in the selected period. Output video codec comes from its probe; legacy records without one are labeled unavailable. Compression alone does not establish visual quality.
- **Worker results:** completions, failures, and bytes recovered in the selected period; observed CPU/GPU counters and configured limits come from heartbeats. Online means a heartbeat was received within 30 seconds. Fleet counts/listings include only workers seen within the last 30 days, independently of the period filter. Online workers appear first, active before idle, with 12 workers per page. The Workers view narrows membership to its selected preset or custom reporting period (both last-seen boundaries); the overview fleet total retains the 30-day maximum. Offline cards show only identity and last seen, without editable controls or live metrics.
- **Job progress:** current assignment and started phase only; its ETA covers that phase. Progress older than one minute is marked stale. Multiple OCR subtasks use mean percentage and the maximum reported ETA; this is not a whole-job or whole-fleet completion forecast.

Charts have descriptive accessible names and hover values. The browser refreshes every five seconds while visible; worker progress is normally published approximately every 15 seconds. The period selector appears in Overview and Workers. **Selected period** groups savings, completions, success, average time, history, codec and activity; **Live / All time** groups current encodes/queue and all-time storage/job totals. Workers show last-seen membership for the selected period within the 30-day fleet maximum, current hardware/tasks, and explicitly labeled period outcomes. Jobs and Library have independent filters. Queries aggregate in PostgreSQL under a consistent read snapshot. The overview samples at most 100 jobs, prioritizing active work and queue entries. The Jobs view queries the entire database by state and literal case-insensitive filename/worker/ID search before pagination, with 25 jobs per page and a matching total. Activity is capped at 30 events, with five visible on the overview. Empty and disconnected states remain explicit, without demo data in production.

New monitoring timestamps are recorded in UTC regardless of server/database timezone. Existing naive historical rows are preserved. If an older deployment wrote local-time timestamps, its historic bucket boundaries may be offset until an operator explicitly corrects those rows using the known original timezone; no automatic conversion guesses that timezone.

## Verification

```bash
go test -race ./...
DASHBOARD_TEST_POSTGRES='host=127.0.0.1 port=5432 user=postgres password=postgres dbname=postgres sslmode=disable' \
  go test -tags=integration -run TestDashboardPostgres -v ./server/repository
npx -y @fission-ai/openspec@latest validate --all --strict
```

The PostgreSQL test creates and removes an isolated schema; its database user needs schema creation permission. Browser verification covers login/logout, no credential exposure, capacity save/pause and worker isolation, period switching, job search/filtering, and mobile overflow. Screenshot previews use separate local test fixtures, never production dashboard data.


## Bidirectional worker UI synchronization

Both the fleet dashboard and local worker controls persist the same per-name settings. The last committed write wins. A local UI save updates desired server values immediately; observed limits and pause state arrive in the next five-second heartbeat, then the fleet's five-second refresh (normally within about ten seconds). Server changes reach local status on the next worker poll; the local web UI refreshes every two seconds. Offline cards show basic identity/last seen; controls and hardware details return after reconnection.

The standalone worker web UI has separate CPU/GPU controls. CLI equivalent: `transcoderd-worker control set-encoder-jobs 2 3`. Saving `0 0` holds the queue; positive limits resume queued starts while retaining RAM and individual holds. `control set-max-jobs N` remains the single-class compatibility control. Failed persistence leaves local limits unchanged. Unsaved form edits are retained until the operator saves them.

Queue stop, RAM pause, individual job pause, finishing and maintenance are distinct observations. RAM pause keeps configured limits and memory allocations. The fleet labels suspended jobs and preserves their progress without presenting their ETA as live. Polling does not reopen maintenance/finish holds. Managed workers restore the persisted server limit on startup and keep local service defaults only when no setting exists, preventing restart from overwriting a newer fleet edit.

Windows continues supervising separate named CPU/GPU profiles (for example `pc-cpu` and `pc-gpu`), each shown as a worker row. These keep their fixed encoder class and limit of 64; the fleet shows only the supported class input and relevant CPU or GPU metrics, while showing shared RAM for both. The unsupported class is submitted as zero and incompatible writes are rejected. Labels identify the shared host so the two profiles are not mistaken for two machines. The native UI's displayed profile capacities follow observed worker status, including fleet changes, while its saved resume values remain available when both profiles are stopped. Standalone workers can still run both classes in one process.

## Install as a PWA

The independent frontend is an installable Progressive Web App. Open `/dashboard/`, then choose **Install app** when your browser offers it, or use the browser's app-install menu. On iPhone/iPad use Safari's **Share → Add to Home Screen**. The installed app launches in its own window with TranscoderD branding and uses the same token/session login as the website. Installation uses the frontend service; it does not expose an additional browser port.

Use HTTPS for a deployed installation. For local review `http://localhost:18080/dashboard/` works without a certificate; localhost/loopback is the development exception to the secure-origin requirement. Browser installation availability varies; unsupported browsers keep the normal website experience. See [MDN's installation requirements](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Making_PWAs_installable).

The service worker is scoped to `/dashboard/`. Its persistent cache contains only an explicit list of public build assets: scripts/styles, manifest, icons and the generic offline page. It never stores the authenticated dashboard HTML, login responses, cookies/tokens, statistics, worker/job records, mutations or video bytes. Opening the app offline shows **Reconnect**, rather than private historical data. Monitoring, video playback and management require the live server. Failed requests are not replayed when connectivity returns; reconnecting checks the session again. Mobile layouts retain a visible logout control.

Changed embedded assets produce a new content-derived worker/cache version. Activation removes only older caches belonging to this dashboard. Updates do not force a reload during edits, playback or intake: use **Update ready · Reload** when convenient. No push notifications or background job submission are enabled.

Run `python frontend/web/dashboard_browser_test.py` with Python Playwright, Chromium and FFmpeg installed for reproducible synthetic browser checks of filtered job pages, native playback/seeking, worker profile controls, offline cards, scanner provenance and mobile identity. Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` when reusing an existing Chromium binary.

## Remote RAM and queue controls

Online workers advertising `controlVersion: 1` expose two independent buttons:

- **Pause in RAM / Resume encodes** suspends/resumes owned active encodes using the worker's existing RAM control. Configured limits stay unchanged, RAM/VRAM remain allocated and individual pauses survive Resume.
- **Stop after current / Resume queue** lets active encodes finish while holding new assignments and queued starts. Resume restores the last positive CPU/GPU pair (or legacy limit) without clearing RAM or individual holds. Saving positive limits also restores the queue explicitly.

Commands travel through worker settings polling, usually within five seconds; the dashboard shows pending until the worker publishes its resulting telemetry and acknowledges the exact command ID. OS/control failures appear as failed with a reason. Remote actions require an upgraded online worker and are disabled during maintenance; no remote PC shutdown is exposed. A superseded acknowledgement cannot complete a newer command, and an acknowledgement retry never replays an already-applied control over a later local action.

Migration 010 adds command storage and positive resume-limit snapshots. The server persists positive local/fleet settings so queue resume survives page refresh and process restart. Already-stopped legacy workers with no positive snapshot need an explicit positive Save limits once; no resume capacity is guessed. Deploy the server migration before the updated workers/frontend. Older workers retain capacity controls but require an update for remote RAM/queue actions.
