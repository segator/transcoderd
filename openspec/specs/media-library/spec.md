# Media library

## Purpose

Provide authenticated rooted media discovery, metadata-based selection previews and embedded playback for source and encoded files without changing the media.

## Requirements

### Requirement: Rooted authenticated media libraries
The dashboard SHALL expose the scheduler source root and optional named web.mediaRoots (empty by default) as navigable libraries. Additional roots SHALL be read-only playback sources. Listing, metadata, original streaming and previews SHALL require dashboard authentication and reject traversal, symlink escapes, hidden staging paths and non-regular media files. Files SHALL NOT be modified by browsing or playback.

#### Scenario: Source and output libraries
- **WHEN** an operator browses source folders or a configured encoded output root
- **THEN** the dashboard lists available folders/videos with rooted file identities and only source-root files are eligible for job intake

#### Scenario: Unauthorized or escaping path
- **WHEN** a caller lacks dashboard authentication or requests a file outside a configured root
- **THEN** the server rejects access without returning media bytes or host filesystem listings

### Requirement: Original and compatible embedded playback
The server SHALL provide original byte-range playback and a browser-compatible H.264/AAC preview with time-based seeking. Preview output SHALL be limited to 720p with web.previewMaxStreams (default two simultaneous streams) and bounded codec threads. Closing/disconnecting the player SHALL stop preview work. The UI SHALL identify preview conversion and support available job sources and completed outputs.

#### Scenario: Browser cannot decode MKV source
- **WHEN** an authenticated operator plays an available MKV source in compatible mode
- **THEN** the embedded player decodes the preview and can restart at a chosen time without changing the source file

#### Scenario: Completed source was removed
- **WHEN** a completed job has a playable output and its source no longer exists
- **THEN** the dashboard offers the output and identifies the unavailable source

#### Scenario: Preview stream limit
- **WHEN** the configured concurrent preview limit is reached
- **THEN** another preview receives an explicit busy response and no extra transcoder is started

### Requirement: Filter previews and cached metadata
Operators SHALL preview recursive folder selections using path/name, size, width, bitrate, codec and job-state filters before queue mutation. Metadata SHALL be cached by root/path and file fingerprint. Width/bitrate lower bounds SHALL be strict and bitrate SHALL use file bits divided by duration. Candidate limits and truncated discovery SHALL be visible. Scan progress and cancellation SHALL be provided.

#### Scenario: Editable movie preset
- **WHEN** the movie preset is selected
- **THEN** it proposes path containing Peliculas, width >1800, total bitrate >4 Mb/s, excludes HEVC/H265/AV1, and orders the largest files first with editable criteria

#### Scenario: Editable series preset
- **WHEN** the series preset is selected
- **THEN** it proposes path containing Series and the same strict width/bitrate thresholds, excludes HEVC/H265, and does not implicitly exclude AV1

#### Scenario: File changed since prior probe
- **WHEN** a file's size or modification time differs from the cached fingerprint
- **THEN** its metadata is reprobed before evaluating metadata filters


### Requirement: Bounded metadata discovery
Scans SHALL default to 1000 probe candidates, allow 1–5000, stop discovery after 100000 videos with an explicit truncation flag, and use at most two concurrent metadata probes. 
#### Scenario: Candidate budget reached
- **WHEN** a folder exceeds the selected candidate budget
- **THEN** the scan reports truncation and does not start unlimited metadata probes

### Requirement: Bounded cancellable library operations
The server SHALL run at most two scan/load operations concurrently, retain at most eight, expire completed handles after 30 minutes, and cancel operations after two hours. Cancellation SHALL stop future work while preserving jobs already queued. Operation handles SHALL be process-local; cached probes and queued jobs SHALL persist.

#### Scenario: Concurrent operation limit
- **WHEN** the active operation limit is reached
- **THEN** the server rejects the new operation as busy rather than starting unlimited work

#### Scenario: Expired selection
- **WHEN** an operator submits an expired preview handle or a handle from before server restart
- **THEN** the server requires a fresh completed preview before loading jobs
