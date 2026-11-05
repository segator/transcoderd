# Media library and visual job loading

The fleet frontend runs separately and reaches the server through its API only. Mount source/output folders and install media tools on the API host; no media or database mounts belong on the frontend host. See [frontend deployment](dashboard.md#deploy-on-a-separate-host).

Open **Library** in `/dashboard/` after logging in with the dashboard token (initial default `encoding123`). The independent frontend serves the page and player; all browsing, media delivery and intake run through the remote server API, without a Plex connection or frontend media mounts. The same session protects directory listings, metadata and every video request.

## Source and encoded libraries

The **Origen** library is `server.scheduler.sourcePath`. It is the only root that can supply new jobs. Source-root finalized outputs are also browsable. To expose a separate existing output mount, configure a named read-only library:

```yaml
web:
  dashboardToken: encoding123
  mediaRoots:
    encoded: /target
  previewMaxStreams: 2
```

CLI equivalent: `--web.mediaRoots encoded=/target --web.previewMaxStreams 2`. Root IDs contain lowercase letters, digits, `_` or `-` (maximum 32 characters); paths must be absolute. `source` is reserved and cannot be overridden. Invalid extra-root entries are ignored. An unavailable mount is labeled unavailable; the server does not silently fall back to a host directory. There are no additional roots by default. Configure mounts in the server container, not the worker.

For the inspected jvdivx deployment, sources already map `/mnt/user/peliculas` to `/source/peliculas` and `/mnt/user/series` to `/source/series`. Existing outputs map `/mnt/user/encode/peliculas` to `/target/peliculas` and `/mnt/user/encode/series` to `/target/series`. Adding the configuration above exposes those output mounts. This implementation does not change the remote container, files, script or queue; rebuilding/redeploying the server is a separate rollout step.

The browser displays paths relative to the selected library. Traversal, hidden paths (including upload staging), symlinks and non-regular files are rejected. Browsing and playback do not modify media. Listings include directories and common video extensions (`mkv`, `mp4`, `m4v`, `avi`, `mov`, `webm`, `mpg`, `mpeg`, `ts`, `m2ts`, `vob`, `ogv`, `wmv`), case-insensitively, with pages of 100 entries. A supported extension does not guarantee that a damaged file can be decoded.

## Prepare, review, load

1. Browse a source folder or enter its relative path. Choose whether to include subfolders.
2. Apply an editable preset, or set path/name contains/excludes, size bounds in GiB, strict minimum width, strict total bitrate and comma-separated codec include/exclude lists. `0` disables a numeric threshold. Codec aliases `h265`/`x265` match `hevc`, and `x264` matches `h264`.
3. Choose **Prepare preview**. The server scans asynchronously without creating jobs; progress, errors and truncation are visible. Cancel stops further analysis.
4. Review the largest matching files first, play them if useful, and select individual eligible rows or **Select eligible files**. Changing filters invalidates the selection and requires a fresh preview.
5. Choose the maximum number of successfully created jobs and **Queue selection**. Skipped/failed candidates do not consume that target. The server continues through the explicitly selected list until it reaches the target or exhausts that list. It reports each outcome.

By default existing queued/active/completed jobs, known completed outputs and files below the server minimum are ineligible. Advanced options permit explicit retries of failed/canceled source jobs; they reuse the job identity/history and current source metadata. Completed jobs cannot be force-requeued through the library. Before each submission the server rechecks path containment, size/mtime and current job state. Changed/missing files and newly existing jobs are skipped; other failures are reported without preventing the remaining selection from loading. Canceling a batch stops further submission; jobs already submitted remain queued.

The maximum candidate-probing budget defaults to 1000 and can be set from 1 to 5000. Cheap path/size/state filters run before probing, then candidates are sorted by size. Width/bitrate/codec filters apply to that bounded candidate list, so a smaller matching file beyond the budget is not implicitly analyzed. Discovery stops after 100000 videos and reports truncation; narrow the folder when this happens. The metadata cache persists in PostgreSQL migration 009, keyed by canonical root identity, relative path, size and nanosecond mtime. Each changed fingerprint is probed again. There are two concurrent probe slots, each with a 15-second timeout.

At most two scan/load operations run concurrently; at most eight are retained. Finished operations expire after 30 minutes and all operation handles disappear on server restart. An operation times out after two hours. Job records and cached metadata persist. Expired previews must be prepared again before loading. No arbitrary shell commands, Plex database mutations or remote script execution are exposed.

## Script-equivalent presets

The presets were checked read-only against `/boot/config/neries/auto-encode.sh` and `auto-encode_p.sh` on jvdivx on 2026-11-03. They reproduce file-selection criteria using server files and ffprobe instead of requiring Plex SQLite access:

| Preset | Path contains | Width | Total bitrate | Excluded codecs |
| --- | --- | --- | --- | --- |
| Películas | `Peliculas` | `>1800` | `>4 Mb/s` | HEVC/H265 and AV1 |
| Series | `Series` | `>1800` | `>4 Mb/s` | HEVC/H265 |
| Todos | Any | Any | Any | None |

Total bitrate means `file_size_bytes × 8 / duration_seconds / 1e6`, including audio/container overhead, matching the scripts' estimate. It is not video-stream bitrate. Unlike the movie preset, the series preset deliberately retains AV1. Path matching is case-insensitive, so actual lowercase container folder names work. Presets use the UI's common default candidate budget of 1000; the old series script's 500-result limit is not an implicit UI limit. The batch target initially shows 500, independently editable. The old movie script considers up to 1000 candidates to reach 500 successful submissions; the new loader counts actual successes in the same way. Filesystem discovery does not filter Plex metadata types or require a Plex library entry, so it also covers videos absent from Plex. Browse or narrow a folder to constrain the library scope.

## Embedded player

Use **View** from the library or scan results to play that specific file. Job tables offer **View source** only for queued jobs and **View output** only after Job completion. Active, failed and canceled jobs have no playback action; requesting their job assets returns HTTP 409. Completed jobs never fall back to the source if the output is missing. Separate legacy outputs remain browsable without a job record. Video comes from the API server's filesystem through the frontend; workers do not stream unfinished encodes.

The dashboard always serves **original file bytes**, including when the selected file is an encoded output. The browser's native controls provide the only playback timeline, seeking, volume and fullscreen. There is no resolution/mode selector or second seek bar. HTTP Range support allows native seeking. Browser container/video/audio support determines playability; an unsupported original displays an explicit error without starting a conversion or changing the file.

The compatible H.264/AAC preview API remains available to existing clients: temporary fragmented MP4 up to 1280×720/24 fps, two codec threads, realtime pacing and time offsets. Its default two-stream limit is configurable with `web.previewMaxStreams`; extra requests return HTTP 429 after up to 500 ms grace for canceled processes to exit. Disconnecting stops FFmpeg. The dashboard player no longer requests this endpoint.

A scan result labeled **Previous encode failed · Retry eligible** refers to the existing encoding job, not a failed scan. It becomes eligible only with **Retry failed jobs** enabled; the retry is submitted only by explicitly queueing the selected files. Analysis failures remain separate per-file scan results.

## Dashboard API

All routes are under `/dashboard/api/media/`, require dashboard authentication, return `Cache-Control: no-store`, and share the same origin/session as the page. POST/DELETE require `X-Dashboard-Request: 1` and the existing same-origin checks. File IDs are opaque rooted identities obtained from listings; callers cannot supply arbitrary absolute paths.

| Method | Relative route | Result |
| --- | --- | --- |
| GET | `roots` | Available libraries and editable presets |
| GET | `browse?root=source&path=Peliculas&offset=0` | One directory page |
| GET | `info/{fileID}` | Cached/probed metadata and eligibility |
| GET / HEAD | `file/{fileID}` | Original bytes; supports Range |
| GET | `preview/{fileID}?start=30` | Compatible MP4 stream from a time offset |
| GET | `jobs/{jobUUID}` | Queued source or completed output assets; HTTP 409 for other states |
| POST | `scans` | HTTP 202 scan operation; body is filter object |
| GET | `operations/{operationUUID}` | Operation state/progress/files/results |
| DELETE | `operations/{operationUUID}` | HTTP 204 cancellation request |
| POST | `batches` | HTTP 202 loader; body `{ "scanId": "…", "ids": ["…"], "target": 500 }` |

Scan filter fields are `root`, `path`, `recursive`, `contains`, `exclude`, `minBytes`, `maxBytes`, `minWidth`, `minBitrateMbps`, `includeCodecs`, `excludeCodecs`, `candidateLimit`, `onlyEligible`, `retryFailed`, `retryCanceled`. Unknown fields are rejected. UI defaults include recursive and only-eligible; direct API clients must send those booleans explicitly. Operations use `running`, `completed`, `failed` or `canceled`; scan matches appear in `files`, per-file failures/submission outcomes in `results`. Selection IDs must belong to a completed source-root scan and be eligible there. Responses identify unavailable/expired resources, invalid selections and busy limits explicitly.
