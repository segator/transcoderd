# Spec Delta

## MODIFIED Requirements

### Requirement: Probe source and output media
The worker SHALL probe a fully downloaded source before encoding and SHALL probe the encoded file before verification. It SHALL use the first video stream reported for the source and determine its duration from the video stream timeline when needed for verification.

#### Scenario: Valid downloaded source
- **WHEN** a source download passes its size and SHA-256 checks
- **THEN** the worker obtains the source video stream, duration, frame rate, audio streams, and subtitle streams before encoding

#### Scenario: Encoded file created
- **WHEN** the encoder exits successfully
- **THEN** the worker probes the output and calculates its SHA-256 checksum before verification and upload

### Requirement: Duration verification
Before upload, the worker SHALL compare source and output video durations against `worker.verifyDeltaTime`, whose registered default SHALL be 60 seconds. When container durations differ by more than the configured threshold, the worker SHALL determine the first video stream's packet timeline before deciding whether to reject the output.

#### Scenario: Duration within limit
- **WHEN** the absolute difference between source and output video durations is at most the configured threshold
- **THEN** the output can advance to upload

#### Scenario: Container duration mismatch caused by non-video streams
- **WHEN** source and output container durations differ by more than the configured threshold
- **AND** the measured first-video-stream durations differ by at most the configured threshold
- **THEN** the output can advance to upload

#### Scenario: Duration beyond limit
- **WHEN** the measured source and output first-video-stream durations differ by more than the configured threshold
- **THEN** verification fails and the worker does not report the job as completed

#### Scenario: Video duration cannot be measured
- **WHEN** a container duration mismatch requires packet-timeline verification
- **AND** the worker cannot determine the first video stream's duration
- **THEN** verification fails with an error identifying the duration measurement failure
