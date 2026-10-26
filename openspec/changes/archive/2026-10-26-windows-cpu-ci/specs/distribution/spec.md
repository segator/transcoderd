## ADDED Requirements

### Requirement: Windows CPU release validation
Windows CI SHALL run native unit tests, bundled media/OCR checks, and a complete SVT-AV1 CPU worker E2E without requiring a GPU. Windows release uploads SHALL occur only after these checks succeed and SHALL attach the versioned portable ZIP and transcoderd-worker-windows-amd64.exe to the matching release.

#### Scenario: Hosted Windows processing
- **WHEN** the Windows CI job runs on a hosted runner without an NVIDIA GPU
- **THEN** the native worker processes the fixture through download, PGS extraction/OCR, SVT-AV1 and AAC encoding, verification, and upload

#### Scenario: Validated Windows release
- **WHEN** a release is created and all Windows checks succeed
- **THEN** its Windows ZIP and platform worker executable are uploaded to that release

#### Scenario: Failed Windows validation
- **WHEN** a native test, bundle check, or Windows CPU E2E fails
- **THEN** the Windows release upload does not run
