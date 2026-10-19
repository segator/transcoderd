# Spec Delta

## MODIFIED Requirements

### Requirement: Video encoder settings
The worker SHALL map the selected source video stream and select an encoder-specific video profile from `worker.ffmpegConfig.videoCodec`. The registered default SHALL be `av1_nvenc`, with preset `p7`, UHQ tuning, VBR CQ `34`, full-resolution multipass, four B-frames with middle references, disabled split encoding, and QP range `0..100`. `libsvtav1` SHALL use preset `3` and CRF `23` by default. Explicit `libx265` SHALL retain preset `medium`, CRF `21`, and profile `main10` by default.

#### Scenario: Default video settings
- **WHEN** no video encoder overrides are supplied
- **THEN** the worker requests the AV1 NVENC profile with the stated defaults

#### Scenario: GPU libraries exposed to the worker
- **WHEN** a compatible NVIDIA GPU and its driver libraries are exposed to the worker container
- **THEN** the default AV1 NVENC job can load the driver and complete encoding

#### Scenario: CPU AV1 selected
- **WHEN** `worker.ffmpegConfig.videoCodec` is `libsvtav1`
- **THEN** the worker requests SVT-AV1 preset `3` and CRF `23`, without NVENC-only options

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

## ADDED Requirements

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
