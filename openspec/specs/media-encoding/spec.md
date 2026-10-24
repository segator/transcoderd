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
The worker SHALL map the selected source video stream and select an encoder-specific video profile from `worker.ffmpegConfig.videoCodec`. The registered default SHALL be `av1_nvenc`, with preset `p7`, UHQ tuning, VBR CQ `38`, full-resolution multipass, four B-frames with middle references, disabled split encoding, and QP range `0..100`. `libsvtav1` SHALL use preset `4` and CRF `25` by default. Explicit `libx265` SHALL retain preset `medium`, CRF `21`, and profile `main10` by default.

#### Scenario: Default video settings
- **WHEN** no video encoder overrides are supplied
- **THEN** the worker requests the AV1 NVENC profile with the stated defaults

#### Scenario: GPU libraries exposed to the worker
- **WHEN** a compatible NVIDIA GPU and its driver libraries are exposed to the worker container
- **THEN** the default AV1 NVENC job can load the driver and complete encoding

#### Scenario: CPU AV1 selected
- **WHEN** `worker.ffmpegConfig.videoCodec` is `libsvtav1`
- **THEN** the worker requests SVT-AV1 preset `4` and CRF `25`, without NVENC-only options

#### Scenario: HEVC rollback selected
- **WHEN** `worker.ffmpegConfig.videoCodec` is `libx265`
- **THEN** the worker requests x265 preset `medium`, CRF `21`, and profile `main10`, without AV1-only options

#### Scenario: Video setting overridden
- **WHEN** a supported encoder-specific quality or preset override is supplied
- **THEN** the worker passes that value to the selected encoder without changing unrelated profile options

#### Scenario: Unsupported encoder or setting
- **WHEN** the configured video encoder or an encoder-specific option is unsupported or incompatible
- **THEN** worker startup fails with a configuration error before requesting jobs

### Requirement: Video pixel format and scaling
The worker SHALL request `p010le` for AV1 NVENC and `yuv420p10le` for SVT-AV1 or x265. It SHALL scale video to at most 1920 pixels wide without enlarging a narrower source, preserving its aspect ratio.

#### Scenario: Wide source
- **WHEN** a source video is wider than 1920 pixels
- **THEN** the output is scaled down with its aspect ratio preserved

#### Scenario: Narrow source
- **WHEN** a source video is narrower than 1920 pixels
- **THEN** the scaling rule does not enlarge its width

#### Scenario: AV1 GPU pixel format
- **WHEN** `av1_nvenc` is selected
- **THEN** the video output requests 10-bit P010 input to the encoder

#### Scenario: AV1 CPU pixel format
- **WHEN** `libsvtav1` is selected
- **THEN** the video output requests 10-bit 4:2:0 input to the encoder

### Requirement: Fixed-profile encode execution
The worker SHALL encode a job using the selected profile without a mandatory separate VMAF analysis or content-dependent retry pass.

#### Scenario: Accepted AV1 job
- **WHEN** an AV1 job reaches the encode phase
- **THEN** the worker starts its selected FFmpeg encode directly and advances to normal output verification after success

### Requirement: Output size verification
Before upload, the worker SHALL measure the encoded file and reject it when its total size exceeds the downloaded source size.

#### Scenario: AV1 result is smaller
- **WHEN** the completed output has fewer bytes than the downloaded source and passes other verification
- **THEN** it can advance to upload

#### Scenario: AV1 result grows
- **WHEN** the completed output has more bytes than the downloaded source
- **THEN** verification fails and the worker does not upload it as a completed job

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
