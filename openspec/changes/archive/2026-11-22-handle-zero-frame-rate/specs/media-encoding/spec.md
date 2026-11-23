## MODIFIED Requirements

### Requirement: Probe source and output media
The worker SHALL probe a fully downloaded source before encoding and SHALL probe the encoded file before verification. It SHALL use the first video stream reported for the source and the container duration for duration comparison. If the reported frame-rate ratio has a zero denominator, it SHALL use the existing 24 fps fallback for progress calculations without terminating the worker.

#### Scenario: Valid downloaded source
- **WHEN** a source download passes its size and SHA-256 checks
- **THEN** the worker obtains the source video stream, duration, frame rate, audio streams, and subtitle streams before encoding

#### Scenario: Encoded file created
- **WHEN** the encoder exits successfully
- **THEN** the worker probes the output and calculates its SHA-256 checksum before verification and upload

#### Scenario: Zero frame-rate denominator
- **WHEN** FFprobe reports an average frame-rate ratio with a zero denominator
- **THEN** normalization uses 24 fps for progress calculations and the worker continues
