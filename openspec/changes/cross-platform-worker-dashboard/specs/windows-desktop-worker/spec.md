## MODIFIED Requirements

### Requirement: Tray monitoring and settings
The tray SHALL show connection and admission status, job phases and progress, queued and downloaded local jobs, and the server queue when available. It SHALL let users configure server URL, token, worker name, encoder, concurrent encodes and admission windows. Tokens SHALL be masked and never returned after saving. It SHALL show the installed version, prominently identify known CPU/GPU profiles and encoders, and provide combinable filters for encoding, downloading, uploading and preparation.

#### Scenario: Display application version
- **WHEN** the tray dashboard is connected to the service
- **THEN** it shows the installed application version

#### Scenario: Identify simultaneous encoder profiles
- **WHEN** CPU and GPU jobs are active
- **THEN** each job card uses a prominent visual badge to identify its profile and configured encoder

#### Scenario: Filter local activity
- **WHEN** a user selects activity filters
- **THEN** the dashboard shows matching local jobs, supports selecting multiple activity types, and provides an all-activity selection

#### Scenario: Running encodes
- **WHEN** one or more jobs are encoding
- **THEN** the tray window shows their identifiers, progress and estimated time remaining when available

#### Scenario: Download in progress
- **WHEN** a source download has provided its filename
- **THEN** the dashboard shows its original filename, downloaded bytes and total size, transfer speed, elapsed time and ETA when available

#### Scenario: Assigned job waiting to download
- **WHEN** an assigned job has not started downloading its source
- **THEN** the dashboard shows the source filename and identifies that it is waiting for download

#### Scenario: FFmpeg encoding progress
- **WHEN** FFmpeg reports encoding progress
- **THEN** the dashboard shows encoded frames, frame rate, encoder speed, elapsed time and ETA when available

#### Scenario: Subtitle preparation
- **WHEN** a job is extracting subtitle tracks or converting PGS subtitles with OCR
- **THEN** the dashboard identifies the active phase as subtitle extraction or subtitle OCR instead of generic preparation

#### Scenario: Prefetched work
- **WHEN** an accepted source has been downloaded and is waiting for an encode slot
- **THEN** the dashboard identifies it as ready to encode separately from active encodes and downloads

#### Scenario: Older server
- **WHEN** the connected server does not expose its global queue view
- **THEN** the dashboard reports that the server queue is unavailable while continuing to display local jobs

#### Scenario: Edit settings
- **WHEN** a user saves valid settings
- **THEN** the service stores them securely, applies a concurrency change without interrupting active work, and applies connection or schedule changes after accepted work drains

#### Scenario: Invalid settings
- **WHEN** a URL, limit, encoder or schedule is invalid
- **THEN** the service reports the error and keeps the active configuration

#### Scenario: Closing the window
- **WHEN** the user closes the tray window
- **THEN** the application remains in the notification area and the worker service continues
