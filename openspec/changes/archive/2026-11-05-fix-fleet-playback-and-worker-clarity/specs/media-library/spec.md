## MODIFIED Requirements

### Requirement: Original and compatible embedded playback
The server SHALL serve original byte ranges and H.264/AAC previews with time seeking, bounded threads, a 720p cap and web.previewMaxStreams (default two). Disconnecting SHALL stop previews. The dashboard SHALL play unchanged files using native controls and one timeline, without quality modes or custom seeking. Job playback SHALL allow queued sources and completed outputs only, with no source fallback for missing outputs. Unsupported browser formats SHALL show an explicit error.

#### Scenario: Browser cannot decode MKV source
- **WHEN** an authenticated client requests a compatible preview of an available MKV source
- **THEN** the server supplies a browser-compatible preview and accepts a chosen restart time without changing the source file

#### Scenario: Native dashboard playback
- **WHEN** an operator opens a video in the dashboard
- **THEN** the original byte-range URL is used with one native playback timeline and no 720p conversion

#### Scenario: Completed source was removed
- **WHEN** a completed job has a playable output and its source no longer exists
- **THEN** the dashboard offers the completed output, labels it as output and never substitutes the source

#### Scenario: Preview stream limit
- **WHEN** the configured concurrent preview limit is reached
- **THEN** another preview receives an explicit busy response and no extra transcoder is started

## ADDED Requirements

### Requirement: Job-state provenance in media selection
Library and scanner results SHALL distinguish previous encoding failure or cancellation from scan/probe errors. An eligible retry SHALL state that selection will retry the prior job; eligibility SHALL continue to require the explicit retry filter. Scanner errors SHALL remain operation or per-file analysis errors.

#### Scenario: Explicit failed-job retry
- **WHEN** Retry failed is enabled and a matching source has a failed encoding job
- **THEN** its result is labeled Previous encode failed and Retry eligible without implying a scan failure or an already-submitted retry
