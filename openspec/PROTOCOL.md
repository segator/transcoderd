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

A worker asks for work with `workerName` and receives a response like:

```json
{"id":"00000000-0000-0000-0000-000000000001","eventID":1}
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

The server streams an assigned source with `Content-Length` and `Content-Disposition`. The worker checks received length, computes SHA-256, and compares it with the checksum route before probing the file. The checksum becomes available after the server closes the download stream.

For upload, the worker sends the encoded bytes with `Content-Length`, `checksum` (hexadecimal SHA-256), and `workerName`. The server checks the current job assignment and compares actual length and checksum before answering `201`. A completion notification follows only after that response. See `BASELINE.md` for the current handling of rejected uploads.


### Authenticated media library

The dashboard API now includes rooted listing, playback and visual intake under `/dashboard/api/media/`. All routes require dashboard auth and no-store responses; POST/DELETE require the dashboard request header and same-origin checks. `GET roots` returns configured roots/presets; `GET browse` accepts `root`, relative `path`, and `offset`; `GET info/{fileID}` returns metadata; `GET/HEAD file/{fileID}` supports original byte ranges; `GET preview/{fileID}?start=seconds` streams compatible fragmented MP4. `GET jobs/{jobUUID}` returns available sources/completed outputs.

`POST scans` accepts the filter object and returns HTTP 202 with an operation UUID. `GET operations/{UUID}` reports progress, `files` and per-file `results`; `DELETE operations/{UUID}` requests cancellation. `POST batches` accepts `scanId`, selected `ids`, and a successful-job `target`, requiring a completed source-root scan. It returns HTTP 202. Operations are bounded, process-local and expiring; previously scheduled jobs survive cancellation/restart. Unknown fields and unreviewed selections are rejected. Rooted IDs cannot grant access outside configured roots. See `docs/media-library.md` for field names, bounds and examples.
