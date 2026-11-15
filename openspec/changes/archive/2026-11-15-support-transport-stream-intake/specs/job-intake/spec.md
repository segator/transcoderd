## MODIFIED Requirements

### Requirement: File discovery
The scheduler SHALL walk the requested path beneath `server.scheduler.sourcePath` and evaluate files against the configured minimum byte size and supported extension list. It SHALL support `.ts` and `.m2ts` files. A directory request SHALL allow multiple files to be evaluated in one response.

#### Scenario: Directory with eligible media
- **WHEN** a requested directory contains files with supported extensions and sufficient size
- **THEN** the scheduler attempts to create a job for each eligible file

#### Scenario: MPEG transport stream source
- **WHEN** a `.ts` or `.m2ts` file meets the existing intake criteria
- **THEN** the server queues it with a provisional `.mkv` destination

#### Scenario: File too small
- **WHEN** a file is smaller than `server.scheduler.minFileSize`
- **THEN** the file and its reason appear in `failed`

#### Scenario: Unsupported extension
- **WHEN** a file's extension is not supported
- **THEN** the file and its reason appear in `failed`
