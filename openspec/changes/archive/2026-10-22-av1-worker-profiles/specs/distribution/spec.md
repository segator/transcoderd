# Spec Delta

## MODIFIED Requirements

### Requirement: AV1-capable FFmpeg in the worker image
The worker image SHALL bundle FFmpeg 9.0.2 with `libsvtav1`, `libaom-av1`, and `av1_nvenc` available. It SHALL retain `libx265` for explicit HEVC rollback and `libfdk_aac` for the default audio setting. When a compatible NVIDIA GPU and driver are exposed to the worker container, its bundled FFmpeg SHALL be able to encode 10-bit AV1 with the worker's default `av1_nvenc` video profile.

#### Scenario: Inspect bundled FFmpeg
- **WHEN** the worker image is built and its FFmpeg encoder list is inspected
- **THEN** FFmpeg reports version 9.0.2 and lists `libsvtav1`, `libaom-av1`, `av1_nvenc`, `libx265`, and `libfdk_aac`

#### Scenario: Encode AV1 on an exposed NVIDIA GPU
- **WHEN** the worker image runs with a compatible NVIDIA GPU and driver
- **THEN** its FFmpeg completes a 10-bit `av1_nvenc` encode using the default worker video profile
