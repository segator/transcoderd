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

Use **View** from the library, preview results or job tables. Job playback offers the existing source and completed output; when the source was removed, the output remains playable and the UI identifies the missing source. Unavailable files show a clear error. Separate legacy outputs are browsable even when no job record exists.

**Compatible** is the default: the server streams temporary H.264/AAC fragmented MP4 at up to 1280×720/24 fps, with stereo audio, realtime input pacing and two video codec threads. The full-duration slider restarts the stream at the chosen second. Closing the dialog, disconnecting or server shutdown terminates FFmpeg; pausing for two seconds releases a compatible stream, and **Reanudar** restarts at the saved position. There are two simultaneous compatible streams by default, configurable through `web.previewMaxStreams`; an extra stream gets HTTP 429 after a short (up to 500 ms) slot-release grace period. Quick seek updates are coalesced before replacing the stream. FFmpeg with libx264/AAC and ffprobe must be present (included in the existing server image). A slow machine may still need fewer concurrent previews.

**Original** serves unchanged bytes with HTTP Range support and native seeking. Playback depends on the browser's container/video/audio support. Use Compatible for MKV or unsupported audio/codecs. Compatible previews are a playback aid, not original-quality compression comparison evidence; select Original only when the browser can decode the source/output directly. Neither mode writes preview files or alters source/output files. Preview resources belong to the server and do not consume configured worker CPU/GPU job slots.

## Dashboard API

All routes are under `/dashboard/api/media/`, require dashboard authentication, return `Cache-Control: no-store`, and share the same origin/session as the page. POST/DELETE require `X-Dashboard-Request: 1` and the existing same-origin checks. File IDs are opaque rooted identities obtained from listings; callers cannot supply arbitrary absolute paths.

| Method | Relative route | Result |
| --- | --- | --- |
| GET | `roots` | Available libraries and editable presets |
| GET | `browse?root=source&path=Peliculas&offset=0` | One directory page |
| GET | `info/{fileID}` | Cached/probed metadata and eligibility |
| GET / HEAD | `file/{fileID}` | Original bytes; supports Range |
| GET | `preview/{fileID}?start=30` | Compatible MP4 stream from a time offset |
| GET | `jobs/{jobUUID}` | Available source/completed output assets |
| POST | `scans` | HTTP 202 scan operation; body is filter object |
| GET | `operations/{operationUUID}` | Operation state/progress/files/results |
| DELETE | `operations/{operationUUID}` | HTTP 204 cancellation request |
| POST | `batches` | HTTP 202 loader; body `{ "scanId": "…", "ids": ["…"], "target": 500 }` |

Scan filter fields are `root`, `path`, `recursive`, `contains`, `exclude`, `minBytes`, `maxBytes`, `minWidth`, `minBitrateMbps`, `includeCodecs`, `excludeCodecs`, `candidateLimit`, `onlyEligible`, `retryFailed`, `retryCanceled`. Unknown fields are rejected. UI defaults include recursive and only-eligible; direct API clients must send those booleans explicitly. Operations use `running`, `completed`, `failed` or `canceled`; scan matches appear in `files`, per-file failures/submission outcomes in `results`. Selection IDs must belong to a completed source-root scan and be eligible there. Responses identify unavailable/expired resources, invalid selections and busy limits explicitly.
