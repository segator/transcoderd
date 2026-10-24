# Spec Delta

## MODIFIED Requirements

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
