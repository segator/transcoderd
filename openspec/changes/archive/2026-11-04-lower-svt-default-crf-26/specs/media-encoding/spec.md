# Media encoding delta

## MODIFIED Requirements

### Requirement: Video encoder settings
The worker SHALL map the selected source video stream and select an encoder-specific video profile from `worker.ffmpegConfig.videoCodec`. The registered default SHALL be `av1_nvenc`, with preset `p7`, UHQ tuning, VBR CQ `38`, full-resolution multipass, four B-frames with middle references, disabled split encoding, and QP range `0..100`. `libsvtav1` SHALL use preset `4` and CRF `26` by default. Explicit `libx265` SHALL retain preset `medium`, CRF `21`, and profile `main10` by default.

#### Scenario: Default video settings
- **WHEN** no video encoder overrides are supplied
- **THEN** the worker requests the AV1 NVENC profile with the stated defaults

#### Scenario: GPU libraries exposed to the worker
- **WHEN** a compatible NVIDIA GPU and its driver libraries are exposed to the worker container
- **THEN** the default AV1 NVENC job can load the driver and complete encoding

#### Scenario: CPU AV1 selected
- **WHEN** `worker.ffmpegConfig.videoCodec` is `libsvtav1`
- **THEN** the worker requests SVT-AV1 preset `4` and CRF `26`, without NVENC-only options

#### Scenario: HEVC rollback selected
- **WHEN** `worker.ffmpegConfig.videoCodec` is `libx265`
- **THEN** the worker requests x265 preset `medium`, CRF `21`, and profile `main10`, without AV1-only options

#### Scenario: Video setting overridden
- **WHEN** a supported encoder-specific quality or preset override is supplied
- **THEN** the worker passes that value to the selected encoder without changing unrelated profile options

#### Scenario: Unsupported encoder or setting
- **WHEN** the configured video encoder or an encoder-specific option is unsupported or incompatible
- **THEN** worker startup fails with a configuration error before requesting jobs

### Requirement: Immutable per-job CPU and GPU profiles
The worker SHALL use separate validated CPU and GPU profiles, with SVT-AV1 and NVENC approved defaults for an unspecified secondary class. Explicit class-specific YAML profiles SHALL be supported. Before encoding, the worker SHALL persist its selected class, profile and resolved SVT CRF in the job context. Resumed work SHALL use that snapshot. Encoder-specific quality and extraArgs SHALL NOT leak into the other class.

#### Scenario: Simultaneous CPU and GPU jobs
- **WHEN** a GPU-primary worker starts one job on CPU and another on GPU
- **THEN** CPU uses its CPU profile and GPU uses its GPU profile concurrently without shared profile mutation

#### Scenario: Restart after profile selection
- **WHEN** a worker restarts after persisting a selected encoder profile
- **THEN** it resumes that encode with the saved class and parameters even if startup settings changed

#### Scenario: Invalid secondary profile
- **WHEN** a CPU profile selects NVENC or a GPU profile selects a CPU codec
- **THEN** worker startup fails with a configuration error

#### Scenario: New CPU job resolves the default before persistence
- **WHEN** a new SVT-AV1 job selects a CPU profile with omitted CRF
- **THEN** its persisted profile contains CRF26 and retains that value after restart or later configuration changes

#### Scenario: Restore a legacy implicit CPU quality
- **WHEN** an existing saved SVT-AV1 profile has CRF0 from before the default change
- **THEN** the worker restores CRF25 rather than applying the new default
