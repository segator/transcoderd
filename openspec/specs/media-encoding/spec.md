# Media encoding and verification

## Purpose

Define the observable video and audio output produced by a worker, the configurable encoding policy, and the checks performed before an encoded file is uploaded.

## Requirements

### Requirement: Probe source and output media
The worker SHALL probe a fully downloaded source before encoding and SHALL probe the encoded file before verification. It SHALL use the first video stream reported for the source and the container duration for duration comparison.

#### Scenario: Valid downloaded source
- **WHEN** a source download passes its size and SHA-256 checks
- **THEN** the worker obtains the source video stream, duration, frame rate, audio streams, and subtitle streams before encoding

#### Scenario: Encoded file created
- **WHEN** the encoder exits successfully
- **THEN** the worker probes the output and calculates its SHA-256 checksum before verification and upload

### Requirement: Video encoder settings
The worker SHALL map the selected source video stream and encode it with `videoCodec`, `videoCRF`, `videoProfile`, and `videoPreset`. The registered defaults SHALL be `libx265`, `21`, `main10`, and `medium`, respectively.

#### Scenario: Default video settings
- **WHEN** no video encoder overrides are supplied
- **THEN** the encoding request uses `libx265`, CRF `21`, profile `main10`, and preset `medium`

#### Scenario: Video setting overridden
- **WHEN** `worker.ffmpegConfig.videoCRF` or another registered video encoder setting is supplied
- **THEN** the worker passes the configured value to the corresponding encoder option

### Requirement: Video pixel format and scaling
The worker SHALL request `yuv420p10le` output and SHALL scale video to at most 1920 pixels wide without enlarging a narrower source, preserving its aspect ratio.

#### Scenario: Wide source
- **WHEN** a source video is wider than 1920 pixels
- **THEN** the output is scaled down with its aspect ratio preserved

#### Scenario: Narrow source
- **WHEN** a source video is narrower than 1920 pixels
- **THEN** the scaling rule does not enlarge its width

### Requirement: Audio stream selection
For each audio language, the worker SHALL select the stream with the most channels; when channel counts are equal, it SHALL select the stream with the greater bit rate. Only selected streams SHALL be mapped for audio encoding.

#### Scenario: Different channel counts
- **WHEN** two audio streams have the same language and different channel counts
- **THEN** the stream with more channels is selected

#### Scenario: Equal channel counts
- **WHEN** two audio streams have the same language and channel count but different bit rates
- **THEN** the stream with the greater bit rate is selected

### Requirement: Audio encoder settings and metadata
The worker SHALL encode each selected audio stream with `audioCodec` and `audioVBR`, whose registered defaults are `libfdk_aac` and `5`. It SHALL set the output audio title from the source language and channel layout.

#### Scenario: Default audio settings
- **WHEN** no audio encoder overrides are supplied
- **THEN** selected audio streams use `libfdk_aac` with VBR quality `5`

#### Scenario: Audio setting overridden
- **WHEN** `worker.ffmpegConfig.audioCodec` or `worker.ffmpegConfig.audioVBR` is supplied
- **THEN** the worker passes the configured codec or VBR value for each selected audio stream

#### Scenario: Audio title
- **WHEN** a selected stream has a language and channel layout
- **THEN** its output title contains both values

### Requirement: Output container and chapters
The worker SHALL produce a Matroska (`.mkv`) file in its job workspace, based on the downloaded source basename with an `-encoded` suffix. It SHALL omit source chapters from that output.

#### Scenario: MP4 source
- **WHEN** the downloaded source basename is `movie.mp4`
- **THEN** the encoded workspace file is named `movie-encoded.mkv`

#### Scenario: Source chapters
- **WHEN** the source contains chapter metadata
- **THEN** the encoded file does not copy those chapters

### Requirement: Additional FFmpeg arguments and thread setting
The worker SHALL pass `worker.ffmpegConfig.extraArgs` to FFmpeg before the input arguments and SHALL use `worker.ffmpegConfig.threads` for FFmpeg's thread option when that nested setting is provided.

#### Scenario: Additional arguments
- **WHEN** `worker.ffmpegConfig.extraArgs` contains FFmpeg options
- **THEN** those options are included in the encoding invocation before the input files

#### Scenario: Explicit encoder threads
- **WHEN** `worker.ffmpegConfig.threads` is set to `4`
- **THEN** the encoding invocation requests four FFmpeg threads

### Requirement: Duration verification
Before upload, the worker SHALL reject an encoded file when the absolute difference between source and output durations exceeds `worker.verifyDeltaTime` seconds. The registered default SHALL be 60 seconds.

#### Scenario: Duration within limit
- **WHEN** the duration difference is at most the configured threshold
- **THEN** the output can advance to upload

#### Scenario: Duration beyond limit
- **WHEN** the duration difference exceeds the configured threshold
- **THEN** verification fails and the worker does not report the job as completed

### Requirement: Encoding failure
The worker SHALL mark the encoding phase as failed if FFmpeg fails or if the encoded file cannot be probed or hashed, and SHALL not upload that output as a completed job.

#### Scenario: FFmpeg exits unsuccessfully
- **WHEN** FFmpeg returns an error or nonzero exit code
- **THEN** the encoding phase fails and the worker reports a job failure
