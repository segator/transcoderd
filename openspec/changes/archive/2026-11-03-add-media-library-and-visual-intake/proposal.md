# Proposal

## Why

Operators currently need Unraid shell scripts and Plex SQL queries to find and enqueue suitable videos, while the dashboard cannot play queued sources or completed outputs. A visual library should support the same selection workflow and preview any available video before submission.

## What Changes

- Add an authenticated folder-based library for the scheduler source root and optional named read-only output roots.
- Embed a player for available files and source/completed assets from job rows, with original byte-range delivery and a browser-compatible seekable H.264/AAC preview stream.
- Add asynchronous filter previews with durable fingerprinted probe caching, selective batch loading, progress/results, and revalidation before queueing.
- Ship editable movie/series presets derived from the inspected Unraid scripts: width >1800, estimated total bitrate >4 Mb/s, descending file size, and their distinct codec exclusions. Queue batches count actual successful jobs toward the requested target.
- Keep all media and operations behind dashboard authentication and reject paths outside the configured roots.

## Capabilities

### New Capabilities

- `media-library`: rooted discovery, metadata caching, streaming, filtering, and selection previews.

### Modified Capabilities

- `operations-dashboard`: library/player/intake interactions and job playback actions.
- `job-intake`: reviewed file selection, successful-job targets, duplicate handling and filtered batch progress.

## Impact

Embedded frontend, dashboard HTTP routes and bounded media processes, scheduler selection API, repository metadata/association queries and additive cache migration, server configuration, documentation, and unit/PostgreSQL/browser playback tests. The source script was read at `/boot/config/neries/auto-encode.sh` and `auto-encode_p.sh` on jvdivx-unraid. No existing remote script, production queue, Plex database, or media file is modified during implementation.
