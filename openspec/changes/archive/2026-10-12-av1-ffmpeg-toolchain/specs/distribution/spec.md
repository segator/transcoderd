# Distribution

## ADDED Requirements

### Requirement: AV1-capable FFmpeg in the worker image
The worker image SHALL bundle FFmpeg 9.0.2 with the `libsvtav1`, `libaom-av1`, and `av1_nvenc` video encoders available. It SHALL retain the `libx265` video and `libfdk_aac` audio encoders required by the current default job settings.
When a compatible NVIDIA GPU and driver are exposed to the worker container, its bundled FFmpeg SHALL be able to encode 10-bit AV1 with `av1_nvenc`.

#### Scenario: Inspect bundled FFmpeg
- **WHEN** the worker image is built and its FFmpeg encoder list is inspected
- **THEN** FFmpeg reports version 9.0.2 and lists `libsvtav1`, `libaom-av1`, `av1_nvenc`, `libx265`, and `libfdk_aac`

#### Scenario: Encode AV1 on an exposed NVIDIA GPU
- **WHEN** FFmpeg in the worker image is asked to encode with a 10-bit pixel format and a compatible NVIDIA GPU is available in the container
- **THEN** `av1_nvenc` produces a decodable 10-bit AV1 video stream
