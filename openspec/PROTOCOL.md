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

## Desktop queue view

`GET /api/v1/jobs/queued` requires the existing bearer token and returns `{ "total": number, "jobs": [{ "id": string, "name": string, "queuedAt": timestamp }] }`. Jobs are ordered by queue time, with at most 100 entries; the count includes all queued jobs. Only the filename is returned, not the server path. This read-only endpoint does not assign work. Older servers do not provide it, and desktop clients display the global queue as unavailable.
