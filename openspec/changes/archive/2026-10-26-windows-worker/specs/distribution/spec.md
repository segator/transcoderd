## ADDED Requirements

### Requirement: Native Windows worker bundle
The build SHALL produce a Windows x64 worker ZIP containing the worker executable, FFmpeg 9.0.2, ffprobe, mkvextract, patched PgsToSrt, a private .NET runtime, native OCR libraries, and pinned Tesseract language data. Running the bundle SHALL require neither Docker, WSL, Go, nor a globally installed .NET runtime.

#### Scenario: Build the portable worker
- **WHEN** the Windows worker bundle build succeeds
- **THEN** a versioned Windows x64 ZIP and a platform-specific worker executable are available in dist

#### Scenario: Required encoding capabilities
- **WHEN** bundled FFmpeg capabilities are inspected
- **THEN** version 9.0.2 and libsvtav1, libaom-av1, av1_nvenc, libx265, and libfdk_aac are available

#### Scenario: End-to-end Windows processing
- **WHEN** a Windows worker receives the integration fixture from a Linux server
- **THEN** it downloads, extracts and OCRs subtitles, encodes AV1 and AAC, verifies, and uploads a completed output retaining the expected languages
