# Design

## Context

See proposal.md. Existing job intake accepts relative files/directories under the scheduler media root. The dashboard has independent auth and an embedded buildless UI. The inspected Unraid movie script filters Plex metadata by width >1800, estimated total bitrate >4 Mb/s, path containing Peliculas, and codec excluding HEVC/H265/AV1; it considers the largest 1000 candidates until 500 submissions actually succeed. The series script uses Series and excludes HEVC/H265 but retains AV1. Production mounts sources below /source and legacy outputs below /target.

## Goals / Non-Goals

**Goals:** browse folders, play sources and completed outputs, review editable script-equivalent filter presets, and load an explicitly reviewed selection with real progress and per-file outcomes.

**Non-Goals:** modifying the remote shell/Plex database, executing arbitrary shell commands, deleting/moving media, automatic production scheduling during development, and claiming preview-transcoded pixels are original-quality evidence.

## Decisions

- Keep the scheduler source root as the queueable library root. Allow named extra roots via web.mediaRoots (for example encoded=/target), which are read-only playback libraries. Open files through os.Root, reject traversal and hidden staging paths, skip symlinks during discovery, and never expose arbitrary host paths. Only source-root files can enter jobs.
- Provide original HTTP byte-range streaming and a compatible fragmented-MP4 preview. The latter uses ffmpeg H.264/AAC at <=720p, realtime input pacing, bounded threads, and a server-wide default of two streams. A full-duration player slider restarts at a requested time, enabling seek on MKV/HEVC/AV1 even when the browser cannot decode the original. Disconnect, closing the player, and shutdown terminate preview processes. Keep credentials in the existing session cookie, not URLs. No external player library or browser extension is required.
- Discover supported files, apply cheap folder/name/size filters, sort largest-first, and probe a bounded candidate list asynchronously (default 1000, maximum 5000) with two probe slots. Bound discovery at 100000 media files and report truncation explicitly. Cache probe results in PostgreSQL by root identity, relative path, size, and modification timestamp; invalidate changed files. Use total file bits/duration to reproduce the scripts' bitrate semantics and strict width/bitrate boundaries.
- Associate files with current job state through batched repository reads. Existing queued/active/completed records and known outputs cannot be submitted by default. Explicit failed/canceled retry options are supported. Presets remain editable and scan preview is non-mutating.
- Keep scan/batch operations in a bounded, expiring process-local registry with cancellation and progress. Require an explicit list of file IDs from a completed preview to enqueue. Validate file fingerprints and current eligibility again; stop at the requested number of actual successful jobs, not attempted HTTP requests. Return scheduled/skipped/failed reasons per file. Serialize intake in the scheduler to avoid concurrent duplicate creation.
- Reuse the existing scheduler's per-file semantics and event history while supplying trusted cached source probes to avoid redundant probing. This also preserves naming, minimum size, and existing force-state rules.

## Risks / Trade-offs

- [First scans need real media metadata] → Persist a fingerprinted cache, cap parallel probing and candidates, expose progress/cancellation, and mark truncated discovery. No new Plex mount or database credential is required.
- [Original MKV may not play natively] → Default to compatible preview, provide an original mode, show its conversion/quality boundary, and include real browser decoding tests.
- [Preview consumes server CPU] → Default two streams and two codec threads each, realtime pacing, cancellation, and no persisted preview files.
- [Files or queue state change after preview] → Revalidate rooted paths, size/mtime and latest job state before each enqueue; count real successes and show changed/missing/duplicate results.
- [Operations expire across restart] → Persist media/job data, make operation progress explicitly temporary, and keep a clear expired-operation UI.

## Migration Plan

Deploy the additive metadata-cache migration with the server, preserving worker protocol and existing data. On jvdivx configure web.mediaRoots.encoded=/target to expose existing output mounts. Existing source-root outputs are available without extra roots. Library/player routes use the same dashboard auth and do not require worker upgrades. Rollback can leave the cache table unused.
