# Spec Delta

## Purpose

Provide authenticated fleet monitoring and individual worker capacity controls using measured job outcomes and live worker progress.

## ADDED Requirements

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
The dashboard SHALL refresh periodically and show job phase progress and ETA when available, worker freshness, configured CPU/GPU encoder, observed concurrency, and desired per-worker limits. Operators SHALL be able to save a non-negative limit for each named worker. Zero SHALL pause new admission; positive limits SHALL adjust encode concurrency without terminating accepted work.

#### Scenario: Change one worker
- **WHEN** an authenticated operator saves a limit for one worker
- **THEN** the server persists that worker's limit, leaves other workers unchanged, and distinguishes desired from observed capacity until synchronization

#### Scenario: Missing or stale telemetry
- **WHEN** a worker has old heartbeat data or supplies no runtime telemetry
- **THEN** the dashboard labels its state and unavailable measurements rather than inventing CPU/GPU utilization
