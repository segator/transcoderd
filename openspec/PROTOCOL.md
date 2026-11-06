# HTTP and event protocol reference

This reference describes the current `/api/v1` interface. The corresponding behavior contracts are in `specs/job-intake/`, `specs/job-dispatch/`, `specs/events-and-progress/`, and `specs/media-transfer/`. Examples show field spelling, not a promise that every optional field will be present.

## Routes

| Method and path | Purpose | Required inputs | Success | Principal error responses |
| --- | --- | --- | --- | --- |
| `POST /api/v1/job/` | Schedule a source file or directory | Bearer token; JSON `sourcePath` | `200` with `scheduled`, `failed`, `skipped` | `400` for empty path; `500` for processing errors |
| `DELETE /api/v1/job/{jobid}` | Request cancellation | Bearer token; job UUID in path | No response body on a successful handler return | `500` for scheduler errors; see `BASELINE.md` for current cancellation behavior |
| `GET /api/v1/job/request` | Assign the next queued job | Bearer token; `workerName` header | `200` with `id`, `eventID`; `204` if empty | `403` without worker name; `500` on server error |
| `GET /api/v1/worker/settings` | Read the persisted job limit for a worker | Bearer token; `workerName` header | `200` with `maxActiveJobs` integer or `null`, and optional `cpuJobs`/`gpuJobs` pair | `400` without worker name; `500` on server error |
| `PUT /api/v1/worker/settings` | Persist a worker's desired job limit | Bearer token; `workerName` header; JSON non-negative `maxActiveJobs` or atomic `cpuJobs` + `gpuJobs` | `204` | `400` for invalid input; `500` on server error |
| `POST /api/v1/event` | Publish a worker event | Bearer token; JSON event envelope | `200` | `500` for decode or processing errors |
| `GET /api/v1/download?uuid=<id>` | Stream assigned source | `workerName` header and started assignment | `200` with bytes and file headers | `403` for wrong worker or state; `404` for missing source |
| `GET /api/v1/checksum?uuid=<id>` | Read published source SHA-256 | Job UUID | `200` with plain-text hexadecimal SHA-256 | `404` when unavailable |
| `POST` or `PUT /api/v1/upload?uuid=<id>` | Receive encoded output | `workerName`, `checksum`, `Content-Length` headers and started assignment | `201` | `400` for invalid length/checksum; `403` for missing checksum, wrong worker, or wrong state |

Job, event, and worker-settings routes use `Authorization: Bearer <web.token>`. Transfer routes are not covered by that middleware in the current code. This is documented as a security gap in `BASELINE.md`.

Workers use the settings routes with their configured `worker.name`. A successful PUT persists that worker's desired maximum, including `0`; a later GET returns it until another PUT changes it. A `null` value means the worker has never saved a runtime limit and should use its configured startup maximum. The worker control command remains local and writes the desired value through these authenticated routes before reporting success. Running workers also poll this value before each normal admission evaluation, approximately every five seconds, including while paused. Failed synchronization skips admission for that cycle without interrupting accepted work. The authenticated dashboard can save the same desired limit for any named worker.

### Independent encoder settings

`PUT /api/v1/worker/settings` and `PUT /dashboard/api/workers/{worker}/settings` accept `{"cpuJobs":2,"gpuJobs":3}` as one atomic update. Both values must be present, non-negative integers, with a sum that fits the server integer range. Incomplete pairs, mixed non-null `maxActiveJobs` and pair, invalid values, and overflow return `400` without changing the stored setting. A legacy `{"maxActiveJobs":N}` write clears both class overrides.

A modern worker sends `workerCapabilities: independent-encoders` on settings GET. For the pair above it receives `{"maxActiveJobs":5,"cpuJobs":2,"gpuJobs":3}`. Without that capability it receives aggregate `maxActiveJobs: 0` plus the same pair, so an older client ignores the unknown fields and safely pauses admission. Existing single-limit GET responses retain their previous shape. Modern clients use the pair, not the aggregate, to configure the two encoder classes.

## Dashboard routes

Dashboard credentials are separate from the worker API. `/` redirects to `/dashboard/`; anonymous dashboard requests redirect to `/dashboard/login`. The login page and local JS/CSS assets contain no operational data. All dashboard responses use `Cache-Control: no-store`.

| Route | Input and authentication | Response |
| --- | --- | --- |
| `GET /dashboard/login` | Public login page | `200` HTML; authenticated visitors redirect to dashboard |
| `POST /dashboard/api/login` | JSON `{"token":"<web.dashboardToken>"}`; `X-Dashboard-Request: 1`; same origin | `204` and HttpOnly SameSite=Strict session cookie; `401` invalid token; `400` invalid JSON; `403` cross-origin or missing request header |
| `POST /dashboard/api/logout` | Dashboard session or dashboard Bearer token; request header and same origin | `204`; revokes browser session and clears cookie |
| `GET /dashboard/api/stats?hours=168` | Dashboard session or `Authorization: Bearer <web.dashboardToken>` | `200` dashboard JSON; supported hours `24`, `168` (default), `720`; `400` invalid period; `401` unauthenticated; `500` read failure |
| `PUT /dashboard/api/workers/{worker}/settings` | Dashboard authentication; request header and same origin; JSON `{"cpuJobs":2,"gpuJobs":3}` (legacy single limit also accepted) | `204` persists desired limit; `400` invalid name or non-negative integer; `401` unauthenticated; `403` cross-origin; `500` persistence failure |

Login and worker-setting JSON bodies are limited to 4096 bytes and must contain a single object with recognized fields. Browser sessions expire after 12 hours, are revoked on logout/server restart, and use Secure cookies when the server uses TLS or `web.domain` is HTTPS. Up to 1024 active sessions are retained; excess logins receive `503`. Set a distinct `web.dashboardToken`; it does not grant worker API access unless an operator independently configures both tokens to the same value.

Statistics contain `generatedAt`, `hours`, `summary`, `timeline`, `workers`, `jobs`, `codecs`, and `activity`. Summary counts and finalized byte totals cover all current jobs. Completion/failure counts with `InRange`, average elapsed processing time, timeline, codec groups, activity, and per-worker outcomes cover the selected rolling period. Timeline bucket boundaries use UTC; the browser labels dates in local time. Job details are limited to 100 (active first, then queued, then recent outcomes) and activity to 30 events. Compression compares paired positive source/output sizes only for `Job/completed`; `FFMPEG/completed` is still active. `savedBytes` is the signed difference and compression is weighted by total bytes. Missing durations/telemetry are unavailable rather than estimated.

Workers include `name`, `ip`, nullable `lastSeen`, `online`, nullable `runtime`, nullable `desiredMax`, nullable `desiredCPUJobs`/`desiredGPUJobs`, and period result statistics. Runtime `capacityVersion: 2` identifies dual-capable workers and adds `cpuJobs`, `gpuJobs`, `activeCPUEncodes`, `activeGPUEncodes`, `cpuMaxEncodes`, `gpuMaxEncodes`, `cpuEncoder`, and `gpuEncoder`. Per-class max encodes describe effective drain targets, which can remain positive while desired jobs is zero. Aggregate fields remain for compatibility. FFMPEG phase messages identify its chosen encoder class and codec. Online means a heartbeat was received within 30 seconds; the desired limit can differ from runtime until the next worker synchronization and heartbeat. Job progress includes `id`, `phase`, `percent` clamped to 0–100, `etaSeconds`, `updatedAt`, and `stale` (older than one minute). Progress matches only the current worker and started phase; ETA applies to that phase, not the whole queue. See [dashboard operations](../docs/dashboard.md).

## Job submission

```json
{
  "sourcePath": "shows/season-1",
  "forceCompleted": false,
  "forceFailed": false,
  "forceCanceled": false
}
```

`sourcePath` is resolved relative to the server's configured source root. Force flags apply only when an existing job has the matching state. The response uses arrays or `null` for `scheduled`, `failed`, and `skipped`, depending on whether each collection received entries. A scheduled job uses `id`, `source_path`, `destination_path`, `source_size`, and optionally other job fields. A failed entry includes the original request fields and `error`.

The extension predicate is case-sensitive. It currently accepts `mp4`, `mpg`, `m4a`, `m4v`, `f4v`, `f4a`, `m4b`, `m4r`, `f4b`, `ogg`, `oga`, `ogv`, `wmv`, `wma`, `webm`, `avi`, `flv`, and `mkv` as ordinary extension values. Four configured values contain trailing spaces (`mov `, `ogx `, `asf `, `vob `), so ordinary files with those suffixes do not match. This is an implementation gap to resolve, not an intended product restriction.

## Assignment and event sequence

A worker asks for work with `workerName` and receives the assigned source filename without its server path:

```json
{"id":"00000000-0000-0000-0000-000000000001","eventID":1,"name":"Example Movie.mkv"}
```

The server has already stored that assignment event. The worker uses the returned `eventID` as its starting point, so its first `Job/started` notification has the next number. Every subsequent notification for the job must increment the number by one, across all phases.

A worker event envelope has this shape:

```json
{
  "eventType": "Notification",
  "eventData": {
    "eventTime": "2026-01-01T00:00:00Z",
    "workerName": "encode-node-1",
    "Id": "00000000-0000-0000-0000-000000000001",
    "eventID": 2,
    "notificationType": "Job",
    "status": "started",
    "message": ""
  }
}
```

`eventType` is `Ping`, `Notification`, or `Progress`. Notification phase values are `Job`, `Download`, `MKVExtract`, `PGS`, `FFMPEG`, `JobVerify`, and `Upload`. Job status values include `queued`, `assigned`, `started`, `completed`, `canceled`, and `failed`. A `Ping` carries `eventTime` and `workerName` in `eventData`.

A progress event carries `jobId`, `progressID`, `percent`, `eta`, `notificationType`, and `status` (`progressing`, `done`, or `failure`) alongside event time and worker name. `eta` is serialized from Go `time.Duration` as a JSON number of nanoseconds; PostgreSQL stores seconds.

## File transfer

The server streams an assigned source with `Content-Length` and an RFC 5987 UTF-8 `Content-Disposition` filename. The worker checks received length, computes SHA-256, and compares it with the checksum route before probing the file. The checksum becomes available after the server closes the download stream.

For upload, the worker sends the encoded bytes with `Content-Length`, `checksum` (hexadecimal SHA-256), and `workerName`. The server checks the current job assignment and compares actual length and checksum before answering `201`. A completion notification follows only after that response. See `BASELINE.md` for the current handling of rejected uploads.


### Authenticated media library

The dashboard API now includes rooted listing, playback and visual intake under `/dashboard/api/media/`. All routes require dashboard auth and no-store responses; POST/DELETE require the dashboard request header and same-origin checks. `GET roots` returns configured roots/presets; `GET browse` accepts `root`, relative `path`, and `offset`; `GET info/{fileID}` returns metadata; `GET/HEAD file/{fileID}` supports original byte ranges; `GET preview/{fileID}?start=seconds` streams compatible fragmented MP4. `GET jobs/{jobUUID}` returns available sources/completed outputs.

`POST scans` accepts the filter object and returns HTTP 202 with an operation UUID. `GET operations/{UUID}` reports progress, `files` and per-file `results`; `DELETE operations/{UUID}` requests cancellation. `POST batches` accepts `scanId`, selected `ids`, and a successful-job `target`, requiring a completed source-root scan. It returns HTTP 202. Operations are bounded, process-local and expiring; previously scheduled jobs survive cancellation/restart. Unknown fields and unreviewed selections are rejected. Rooted IDs cannot grant access outside configured roots. See `docs/media-library.md` for field names, bounds and examples.
## Desktop queue view

`GET /api/v1/jobs/queued` requires the existing bearer token and returns `{ "total": number, "jobs": [{ "id": string, "name": string, "size": number, "queuedAt": timestamp }] }`. Jobs are ordered by queue time, with at most 100 entries; the count includes all queued jobs. Only the filename is returned, not the server path. This read-only endpoint does not assign work. Older servers do not provide it, and desktop clients display the global queue as unavailable.


## Windows managed-update control

The optional numeric request `id` correlates browser bridge requests. The
restricted desktop named pipe accepts `status`, `configure`, `set-max-jobs`
and `check-update`. `check-update` queues an immediate release check; it cannot
select an endpoint, key, path, executable or installer arguments. Unknown fields
are rejected. There is no update-disable, postpone or confirmation command.

Desktop status includes an optional `update` object, separate from pending
configuration. It contains `pending`, `phase`, `installedVersion`,
`targetVersion`, `downloaded`, `total`, `lastCheck`, `nextRetry`, `attempts` and
`message`. Byte counts are integers; timestamps use RFC 3339. Phases are
`current`, `pending`, `downloading`, `draining`, `prepared`, `installing`,
`verifying`, `recovering`, `restart-required`, `retrying` and `updated`.
The authenticated candidate, signature, installer URL and server token are never
included in this public status.

While `update.pending` is true, `configure` and `set-max-jobs` return
`A mandatory update is pending. Configuration is locked until the update completes.`
without mutating saved or running configuration. Status and progress remain
readable. The UI locks on unconfirmed service status and retains its last pending
alert across disconnects and reloads. Linux browser dashboards do not expose the
Windows installer lifecycle.


## Worker pause and maintenance drain

Worker status includes `paused`, indicating whether queued FFmpeg starts are
held. Operator `set-max-jobs 0` persists zero, stops new requests and queued
encodes, and applies the configured `pauseMode` to active encodes while verification and uploads finish. Positive
values resume at that encode target. The admission ceiling is the effective
encode target plus one accepted pipeline job per profile, including reservations.
Windows service workers restore persisted per-name settings before admission and keep their service-owned defaults only when no override exists. Paused Windows startup holds restored
encodes too. Internal `drain` and `hold-admission` permit accepted queued work to
finish, resuming any RAM-suspended encoder first, without persisting a change to the user's pause preference. The mandatory
update/configuration lifecycle owns those temporary maintenance transitions.

Local `set-pause-mode` accepts `pauseMode: "finish"` or `"ram"`. Runtime status includes `pauseMode` (default `finish`) and `suspendedEncodes` (actual owned suspended encoding process count); the corresponding `jobs` entries use `state: "paused"`. Existing `activeEncodes` includes the occupied encoding slots, including suspended ones. Desktop public settings include `pauseMode`; it is saved in the protected service configuration. HTTP server worker-settings payloads remain unchanged. No durable process or chunk checkpoints are exposed.

## Individual encoding control

Local `pause-job` and `resume-job` commands carry `jobId` (a job UUID), with CLI forms `control pause-job <UUID>` and `control resume-job <UUID>`. The worker resolves only owned active encoding processes. The desktop service resolves the owning CPU/GPU profile from live snapshots; callers cannot select a PID or trusted profile. Stale jobs and maintenance/pending-reconfiguration controls fail.

The authenticated optional browser dashboard exposes `POST /api/worker/jobs/{jobId}/pause` and `/resume`, protected by its existing dashboard bearer token. Invalid UUIDs return 400, unknown/noncontrollable encodes 404, and rejected transitions 409. The routes return the refreshed worker status on success.

Job status includes `canPause`, `individuallyPaused` and `pausedByWorker`. An individual hold uses RAM pause regardless of the general pause mode, retains its occupied slot, and does not alter admission or limits. General Resume does not clear individual holds. Individual Resume cannot bypass a general hold. Maintenance clears live holds so accepted encodes can finish and blocks further individual mutations.

## Independent queue and RAM controls

Local `set-admission-limit` carries `maxJobs` and persists a finish-active admission limit. Local `pause-encodes` / `resume-encodes` suspend or resume owned encoding processes immediately without changing the admission limit. Runtime status exposes `ramPaused` independently of `maxActiveJobs`; zero limit still identifies a stopped queue. Both holds gate new assignments/starts and releasing either preserves the other and individual job holds.

The optional authenticated dashboard's existing `/api/worker/max-jobs` route now uses the explicit admission operation; `POST /api/worker/encodes/pause` and `/resume` use the RAM operation. The desktop bridge accepts the same RAM commands and saves `ramPaused` separately from its existing `paused` queue preference. Generated profile config carries `worker.ramPaused` and finish zero-limit policy. Both preferences are restored before accepting work, while maintenance temporarily releases live suspensions and preserves the saved preferences.

### Explicit Windows shutdown after accepted work

The restricted desktop pipe accepts `stop-after-current` with boolean
`shutdownAfterCurrent`. Only `true` arms a transient request; omitted/false uses
ordinary queue stop. Desktop status includes `shutdownPending`. Both profiles
must confirm idle with no active jobs, encodes or reserved requests before the
service invokes normal Windows shutdown once. Resume/configuration/maintenance,
worker exit or control loss, and service restart cancel the request. The native
tray handles `confirm-shutdown` locally and returns the user's boolean choice;
this is not a service command. No HTTP power-management endpoint is exposed.
The worker's local `finish-accepted` command closes admission and runs accepted
queued work; the next explicit admission-limit change releases this operator
hold. Maintenance admission holds remain independent.

### Manual Setup preparation

The installer's extracted service helper accepts `--prepare-upgrade-wait`. It
polls the restricted desktop status channel and existing worker `hold-admission`
controls until both profiles report no accepted jobs, queued work, active encodes
or reserved requests. It then invokes `prepare-upgrade` and stops the service.
The immediate `--prepare-upgrade` path remains available for uninstall. Service
preparation drains all connected profiles before returning busy and rejects
missing configured profiles. Setup logs remaining jobs/requests while waiting;
the wait is bounded to 24 hours and ends if its Setup parent process exits.
Neither path force-stops accepted work or arms PC shutdown.


### Worker UI synchronization

Local HTTP `POST /api/worker/max-jobs` accepts either legacy `maxJobs` or the atomic `cpuJobs`/`gpuJobs` pair (total at most 1024); missing, mixed or incomplete settings are rejected. Local CLI `control set-encoder-jobs CPU GPU` shares the same server persistence. Settings writes complete before local runtime changes. Managed startup restores server overrides and keeps local service defaults when absent. Last committed writes win; worker polls and heartbeats run approximately every five seconds, fleet refresh every five seconds, local browser refresh every two seconds.

Runtime heartbeat data adds `paused`, `ramPaused`, `pauseMode`, `suspendedEncodes`, `finishing`, `draining`, `managedProfile` and bounded `jobs` entries (`id`, `phase`, `state`, `individuallyPaused`, `pausedByWorker`). These observations do not overwrite desired limits. RAM and individual holds survive capacity changes. Queue stop holds pending encodes; maintenance/finish allows accepted work to drain without reopening admission. Windows supervised profiles retain their encoder class and support up to 64 slots each; incompatible fleet pair writes return 400. See `docs/dashboard.md`.

## Independent frontend and dashboard session

The server exposes dashboard JSON/media APIs under `/dashboard/api/` and no dashboard HTML or static assets. `transcoderd-frontend` serves `/dashboard/`, `/dashboard/login`, public assets and the PWA on its own origin and proxies an allowlist of dashboard API routes to `frontend.apiURL`. It does not proxy `/api/v1` worker routes. All existing dashboard API paths and payloads remain unchanged.

`GET /dashboard/api/session` returns 204 for a valid dashboard cookie or dashboard bearer token and 401 otherwise, with no operational content and Cache-Control: no-store. The frontend uses this endpoint before serving private HTML. Login/logout and media remain authenticated by the API; frontend mutations additionally validate the frontend browser origin and X-Dashboard-Request before forwarding. Original media ranges, conditional headers, preview offsets and streaming cancellation survive the proxy hop. No media paths are resolved on the frontend host.

## Worker hardware resources

`Ping.runtime.resources` is optional. It carries `sampledAt`, `hostname`, `os`, `architecture`, optional `cpu` (`percent` nullable, `cores`, `scope` host or processor-group), optional `memory` (`usedBytes`, `totalBytes`), `gpuStatus`, `driverVersion` and `gpus`. Each GPU includes UUID/name/PCI bus identity and nullable `percent`, `encoderPercent`, `decoderPercent`, `memoryUsedBytes`, `memoryTotalBytes`, `temperatureC`, `powerWatts` and `powerLimitWatts`. Zero is a measured value; null denotes unavailable. GPU status is available, unavailable or no-devices. The existing runtime JSON persistence and dashboard stats response retain the snapshot unchanged, without a migration or a new endpoint. See [resource semantics](../docs/worker-resources.md).


## OIDC frontend browser boundary

Token mode remains the compatible default. OIDC mode adds frontend-owned `GET /dashboard/auth/login` and `GET /dashboard/auth/callback`. Login redirects to the discovered provider with authorization code, state, nonce and PKCE S256; callback verifies the signed identity and optional groups before server-side API login. The callback URI always uses the configured public origin, never forwarded headers. The API wire protocol stays unchanged.

OIDC browsers receive only encrypted `transcoderd_frontend` and transient `transcoderd_oidc_login` cookies; the former carries an opaque API-owned session for at most twelve hours and the latter expires after ten minutes. Both are HttpOnly SameSite=Lax and Secure under HTTPS. The proxy sends the recovered API session only; it discards browser bearer/legacy credentials and strips upstream cookies. POST `/dashboard/api/login` returns 403 in OIDC mode. Authenticated logout still uses POST `/dashboard/api/logout`, revokes the API session and clears the frontend cookie; it does not terminate the provider session. OIDC auth routes bypass service-worker interception and responses use no-store. The frontend API session endpoint remains 204/401.

## Filtered dashboard jobs and operator identity

`GET /dashboard/api/jobs` requires dashboard authentication and returns `{jobs, total, offset, limit}`. `state` accepts all (default), active, queued, completed, failed or canceled. `search` is a trimmed literal case-insensitive substring of source path, worker name or UUID (at most 256 bytes). `offset` is nonnegative (default 0); `limit` is 1–100 (default 25). Invalid values return 400. Filtering/counting precede pagination under one read snapshot; current-assignment progress is included. Statistics retain global totals and their existing overview sample.

Dashboard worker membership includes only heartbeats within 30 days of generatedAt; online still means 30 seconds. Online/active workers sort first. Historical records and desired settings remain stored.

The frontend-owned `GET /dashboard/api/identity` returns `{mode, name?, username?, email?}` after validating the backend session (401 for invalid/revoked, 503 for unavailable). Mode is token or oidc. Only verified bounded OIDC display claims are included; provider tokens and the API session never appear. The existing session validation route stays 204/401. Both jobs and identity are no-store and bypass PWA caching.

`GET /dashboard/api/media/jobs/{id}` returns only a queued source or completed output; active, failed and canceled jobs return 409. A missing completed output produces an empty files array even when the source exists. Direct rooted library file playback remains available independently. The dashboard uses original byte-range files and native browser controls; the existing compatible preview API is retained for other clients.

## Acknowledged remote worker controls

`POST /dashboard/api/workers/{worker}/control` accepts worker-wide `{ "action": "pause-encodes" | "resume-encodes" | "stop-after-current" | "resume-queue" }` with dashboard authentication and same-origin protections. HTTP 202 returns a command `{id, action, state:"pending", cpuJobs?, gpuJobs?, maxActiveJobs?}`. Unsupported, offline or maintenance workers return 409; invalid actions/fields return 400. Dashboard workers include an optional `control` snapshot with pending/applied/failed state and bounded error text. Offline cards retain their minimal presentation.

Workers advertise `runtime.controlVersion: 2` and send `workerControlVersion: 2` on the existing settings GET. Version 1 remains supported for worker-wide controls; individual commands are delivered only to version 2. Only capable clients receive the optional pending `control` field; legacy clients keep their previous response shape. After applying via local controls and publishing resulting telemetry, the worker POSTs `{id, error?}` to `/api/v1/worker/control/ack` with the worker bearer credential and `workerName`. Success is 204; a superseded ID is 409 and cannot acknowledge the current command. Errors are limited to 512 bytes. A single durable per-worker slot represents the latest command; failed outcomes remain visible until another command replaces them.

RAM commands do not mutate capacity. Queue stop persists zero limits before holding starts with finish semantics, preserving explicit RAM/individual holds. Positive settings are retained as the queue-resume snapshot; resume persists/restores that snapshot. Settings polling and commands share worker serialization, and an applied ID/result is retained while retrying a failed acknowledgement. Maintenance remains authoritative. The Workers view filters lastSeen by selected reporting hours inside the 30-day fleet maximum; card plots retain only bounded current-tab samples.


## Reporting ranges and individual remote encodes

`GET /dashboard/api/stats` keeps `hours=24|168|720` (default 168), or accepts `start` and `end` RFC3339 timestamps without `hours`. Custom boundaries are start-inclusive and end-exclusive, ordered and at most 3660 days apart. Invalid/mixed/incomplete parameters return 400. The response adds `range:{start,end}` and `period` (completed/failed, measured sourceBytes/targetBytes/savedBytes/compressionPercent, averageSeconds). Existing `summary` all-time fields and preset compatibility remain; all selected-period aggregates use both boundaries. Current jobs and 30-day fleet presence remain live. Hourly buckets apply to ranges up to 24 hours, daily otherwise.

Worker runtime job snapshots add `name`, `canPause` and current-phase `progress` entries. These come from local worker status and do not depend on the overview's bounded job list. `POST /dashboard/api/workers/{worker}/control` additionally accepts `{action:"pause-job"|"resume-job",jobId:"<UUID>"}`. A command and dashboard command snapshot include `jobId`; migration 011 persists it. Missing/invalid UUID or a jobId on a worker-wide action returns 400; old, stale, offline, maintenance, non-owned or non-controllable worker observations return 409. At application the worker revalidates its owned process and reports an explicit failure if the encode has already left. Individual resume preserves all other holds. Existing same-origin, credentials, acknowledgement and supersession protections apply.


## Worker software version telemetry

Worker heartbeat `runtime.version` optionally carries the running binary's injected release string. Dashboard worker `runtime` returns it through existing authenticated stats; the JSON runtime column needs no migration. Older heartbeats without this field remain accepted and omit it in encoded runtime. The frontend shows the reported value and labels offline values last reported; absent/empty versions are unavailable, never inferred from `controlVersion` or the API's release. No GitHub request or update command is triggered by viewing a version.
