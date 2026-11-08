## MODIFIED Requirements

### Requirement: GPU hardware decoding
For eligible AV1 NVENC jobs, the worker SHALL request NVIDIA hardware decoding and 10-bit AV1 output. Ineligible jobs SHALL use software decoding. Recognizable decoder initialization failures before encoding starts SHALL trigger one software-decoding retry. CPU encoder jobs SHALL retain software decoding.

#### Scenario: Eligible GPU job
- **WHEN** an AV1 NVENC job receives compatible H.264 or HEVC video with both dimensions no larger than 2000 pixels
- **THEN** the worker requests NVIDIA decoding and 10-bit AV1 encoding

#### Scenario: Ineligible or unsupported input
- **WHEN** a GPU job has a dimension above 2000, missing metadata, an unsupported source format, an invalid YUV color matrix, or custom input arguments
- **THEN** it uses software decoding and preserves the existing scaling and output format policies

#### Scenario: Decoder initialization failure
- **WHEN** the hardware decoder fails to initialize before encoding begins
- **THEN** the worker retries using software decoding

#### Scenario: CPU encoder
- **WHEN** a job uses SVT-AV1 or x265
- **THEN** the worker uses software video decoding

### Requirement: Video pixel format and scaling
The worker SHALL produce 10-bit 4:2:0 AV1 NVENC output and SHALL request `yuv420p10le` for SVT-AV1 or x265. It SHALL omit the resizing filter when both known source dimensions do not exceed 2000 pixels. Otherwise it SHALL scale video to at most 1920 pixels wide without enlarging a narrower source, preserving its aspect ratio.

#### Scenario: Wide source
- **WHEN** a source video is wider than 2000 pixels
- **THEN** the output is scaled down with its aspect ratio preserved

#### Scenario: Narrow source
- **WHEN** a source video is narrower than 1920 pixels
- **THEN** the scaling rule does not enlarge its width

#### Scenario: AV1 GPU pixel format
- **WHEN** `av1_nvenc` is selected
- **THEN** the video output is 10-bit 4:2:0

#### Scenario: AV1 CPU pixel format
- **WHEN** `libsvtav1` is selected
- **THEN** the video output requests 10-bit 4:2:0 input to the encoder

#### Scenario: No resizing needed
- **WHEN** both source dimensions are known and at most 2000 pixels
- **THEN** the worker does not request a video resizing filter
