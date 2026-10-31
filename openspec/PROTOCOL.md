# HTTP and event protocol reference

This reference describes the current `/api/v1` interface. The corresponding behavior contracts are in `specs/job-intake/`, `specs/job-dispatch/`, `specs/events-and-progress/`, and `specs/media-transfer/`. Examples show field spelling, not a promise that every optional field will be present.

## Routes

| Method and path | Purpose | Required inputs | Success | Principal error responses |
| --- | --- | --- | --- | --- |
| `POST /api/v1/job/` | Schedule a source file or directory | Bearer token; JSON `sourcePath` | `200` with `scheduled`, `failed`, `skipped` | `400` for empty path; `500` for processing errors |
| `DELETE /api/v1/job/{jobid}` | Request cancellation | Bearer token; job UUID in path | No response body on a successful handler return | `500` for scheduler errors; see `BASELINE.md` for current cancellation behavior |
| `GET /api/v1/job/request` | Assign the next queued job | Bearer token; `workerName` header | `200` with `id`, `eventID`; `204` if empty | `403` without worker name; `500` on server error |
| `GET /api/v1/worker/settings` | Read the persisted job limit for a worker | Bearer token; `workerName` header | `200` with `maxActiveJobs` integer or `null` | `400` without worker name; `500` on server error |
| `PUT /api/v1/worker/settings` | Persist a worker's desired job limit | Bearer token; `workerName` header; JSON `maxActiveJobs` non-negative integer | `204` | `400` for invalid input; `500` on server error |
| `POST /api/v1/event` | Publish a worker event | Bearer token; JSON event envelope | `200` | `500` for decode or processing errors |
| `GET /api/v1/download?uuid=<id>` | Stream assigned source | `workerName` header and started assignment | `200` with bytes and file headers | `403` for wrong worker or state; `404` for missing source |
| `GET /api/v1/checksum?uuid=<id>` | Read published source SHA-256 | Job UUID | `200` with plain-text hexadecimal SHA-256 | `404` when unavailable |
| `POST` or `PUT /api/v1/upload?uuid=<id>` | Receive encoded output | `workerName`, `checksum`, `Content-Length` headers and started assignment | `201` | `400` for invalid length/checksum; `403` for missing checksum, wrong worker, or wrong state |

Job, event, and worker-settings routes use `Authorization: Bearer <web.token>`. Transfer routes are not covered by that middleware in the current code. This is documented as a security gap in `BASELINE.md`.

Workers use the settings routes with their configured `worker.name`. A successful PUT persists that worker's desired maximum, including `0`; a later GET returns it until another PUT changes it. A `null` value means the worker has never saved a runtime limit and should use its configured startup maximum. The worker control command remains local and writes the desired value through these authenticated routes before reporting success.

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
Windows service workers synchronize their service-owned targets before admission
instead of loading older server overrides. Paused Windows startup holds restored
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
