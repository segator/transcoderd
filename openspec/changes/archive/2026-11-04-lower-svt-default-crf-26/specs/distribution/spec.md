# Distribution delta

## MODIFIED Requirements

### Requirement: AV1-capable FFmpeg in the worker image
The worker image SHALL bundle FFmpeg 9.0.2 with SVT-AV1 4.3.0 providing `libsvtav1`, and with `libaom-av1` and `av1_nvenc` available. It SHALL retain `libx265` for explicit HEVC rollback and `libfdk_aac` for the default audio setting. When a compatible NVIDIA GPU and driver are exposed to the worker container, its bundled FFmpeg SHALL be able to encode 10-bit AV1 with the worker's default `av1_nvenc` video profile.

#### Scenario: Inspect bundled FFmpeg
- **WHEN** the worker image is built and its FFmpeg encoder list is inspected
- **THEN** FFmpeg reports version 9.0.2 and lists `libsvtav1`, `libaom-av1`, `av1_nvenc`, `libx265`, and `libfdk_aac`

#### Scenario: Encode AV1 on an exposed NVIDIA GPU
- **WHEN** the worker image runs with a compatible NVIDIA GPU and driver
- **THEN** its FFmpeg completes a 10-bit `av1_nvenc` encode using the default worker video profile

#### Scenario: Verify bundled CPU encoder version
- **WHEN** bundled FFmpeg performs a CPU AV1 validation encode using preset 4, CRF26 and 10-bit 4:2:0 input
- **THEN** its encoder log identifies SVT-AV1 4.3.0 and the output probes as 10-bit AV1 and passes full decoding

### Requirement: Native Windows worker bundle
The build SHALL produce a Windows x64 worker ZIP containing the worker executable, FFmpeg 9.0.2 with SVT-AV1 4.3.0, ffprobe, mkvextract, patched PgsToSrt, a private .NET runtime, native OCR libraries, and pinned Tesseract language data. Running the bundle SHALL require neither Docker, WSL, Go, nor a globally installed .NET runtime.

#### Scenario: Build the portable worker
- **WHEN** the Windows worker bundle build succeeds
- **THEN** a versioned Windows x64 ZIP and a platform-specific worker executable are available in dist

#### Scenario: Required encoding capabilities
- **WHEN** bundled FFmpeg capabilities are inspected
- **THEN** version 9.0.2 and libsvtav1, libaom-av1, av1_nvenc, libx265, and libfdk_aac are available

#### Scenario: End-to-end Windows processing
- **WHEN** a Windows worker receives the integration fixture from a Linux server
- **THEN** it downloads, extracts and OCRs subtitles, encodes AV1 and AAC, verifies, and uploads a completed output retaining the expected languages

#### Scenario: Verify native Windows CPU encoder version
- **WHEN** the native bundled FFmpeg performs a CPU AV1 validation encode using preset 4, CRF26 and 10-bit 4:2:0 input
- **THEN** its encoder log identifies SVT-AV1 4.3.0 and the output probes as 10-bit AV1 and passes full decoding
