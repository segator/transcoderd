# operations-dashboard Specification

## Purpose

Provide authenticated fleet monitoring and individual worker capacity controls using measured job outcomes and live worker progress.

## Requirements

### Requirement: Dashboard authentication
The server SHALL serve a login page at `/dashboard/login` and protect the dashboard and its data and controls with a separate static `web.dashboardToken`, default `encoding123`. Login SHALL create an expiring HttpOnly SameSite browser session. The dashboard token SHALL NOT authorize worker API routes.

#### Scenario: Anonymous access
- **WHEN** an unauthenticated visitor opens the dashboard or requests its data
- **THEN** the page redirects to login and data requests return HTTP 401 without operational data

#### Scenario: Valid login and logout
- **WHEN** a visitor supplies the configured dashboard token
- **THEN** the server creates a session that permits dashboard access until expiry or browser logout

#### Scenario: Invalid credentials
- **WHEN** a visitor supplies an incorrect or empty token
- **THEN** login returns HTTP 401 without creating a session

### Requirement: Protected dashboard mutations
Dashboard mutations SHALL require authentication and same-origin request protection. Responses SHALL prevent operational data caching. Credentials SHALL NOT be included in page content or request URLs.

#### Scenario: Cross-origin mutation
- **WHEN** a request attempts login, logout, or a settings change from another origin
- **THEN** the server rejects the request without changing settings

### Requirement: Useful fleet measurements
The dashboard SHALL show current queued, active, completed, failed, and canceled job counts; finalized input/output sizes and weighted storage savings; completion and failure history; worker statistics; and bounded recent jobs and activity. Period filters SHALL support 24 hours, 7 days, and 30 days. Terminal job counts SHALL use Job-level outcomes, not intermediate phase completion.

#### Scenario: Intermediate phase completion
- **WHEN** a job completes its FFMPEG phase but has not completed verification and upload
- **THEN** it remains active and its output does not contribute to finalized compression savings

#### Scenario: Weighted compression
- **WHEN** finalized jobs have valid source and target sizes
- **THEN** aggregate compression uses the sum of their sizes, and empty datasets show an empty state without fabricated measurements

### Requirement: Live monitoring and individual capacity
The dashboard SHALL refresh periodically and show job phase progress and ETA when available, worker freshness, CPU/GPU encoders, active counts, and independent desired limits. Operators SHALL save CPU and GPU limits atomically per worker. Zero SHALL disable that class for new unassigned encodes; both zero SHALL pause admission. Accepted work SHALL continue.

#### Scenario: Change one worker
- **WHEN** an authenticated operator saves CPU and GPU limits for one worker
- **THEN** the server persists that worker's pair, leaves other workers unchanged, and distinguishes desired from observed capacity until synchronization

#### Scenario: Missing or stale telemetry
- **WHEN** a worker has old heartbeat data or supplies no runtime telemetry
- **THEN** the dashboard labels its state and unavailable measurements rather than inventing CPU/GPU utilization

### Requirement: Visual media exploration and job playback
The authenticated dashboard SHALL offer folder navigation, selectable video results, editable filter presets, an embedded player and explicit reviewed job submission. Job rows SHALL offer playback of available sources and completed outputs. The UI SHALL show scan/load progress, actual successes, skipped/failed reasons, cancellation, empty/error states and unavailable files without invented data.

#### Scenario: Browse and play queued source
- **WHEN** an operator opens a queued job's available source
- **THEN** its video can be played inside the dashboard without worker assignment or a worker credential

#### Scenario: Review before loading jobs
- **WHEN** an operator changes selection filters
- **THEN** no jobs are added until a completed preview and explicit file selection are submitted

### Requirement: Observe worker UI changes and pause states
Worker heartbeats SHALL include observed queue/RAM holds, suspended encode count, individual job pause state and finishing/draining state. The fleet dashboard SHALL distinguish these from offline, schedule and desired-limit acknowledgement states. Observed updates SHALL appear through its normal refresh without manually reloading. Paused encodes SHALL retain progress without a live-running ETA claim.

#### Scenario: Individual encode paused locally
- **WHEN** an operator pauses an active encode in the worker UI
- **THEN** the next heartbeat and fleet refresh identify that encode as paused without treating it as failed or completed

#### Scenario: RAM pause differs from queue stop
- **WHEN** an operator toggles RAM hold while positive limits remain configured
- **THEN** the dashboard identifies RAM pause and suspended encodes independently of the desired CPU/GPU values

#### Scenario: Remote change reaches local worker UI
- **WHEN** the fleet dashboard saves new limits and the worker synchronizes them
- **THEN** local worker controls and Windows profile status show the observed limits without restarting

