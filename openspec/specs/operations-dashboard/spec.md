# operations-dashboard Specification

## Purpose

Provide authenticated fleet monitoring and individual worker capacity controls using measured job outcomes and live worker progress.

## Requirements

### Requirement: Dashboard authentication
The independent frontend SHALL serve `/dashboard/login` and private dashboard pages. The server SHALL authenticate dashboard API requests using separate static `web.dashboardToken`, default `encoding123`, and expose authenticated `GET /dashboard/api/session` returning 204 or 401. Login SHALL create an expiring HttpOnly SameSite browser session. The dashboard token SHALL NOT authorize worker API routes.

#### Scenario: Anonymous access
- **WHEN** an unauthenticated visitor opens the dashboard or requests its data
- **THEN** the frontend page redirects to login and server data/session requests return HTTP 401 without operational data

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

### Requirement: Installable operations dashboard
The independent frontend SHALL expose an embedded application manifest with stable dashboard identity, dashboard-scoped standalone launch, branded 192px/512px and maskable icons, theme metadata and Apple touch icon. Login and dashboard pages SHALL link installation metadata. Where supported, installation controls SHALL follow browser availability and disappear after installation. Installation SHALL retain normal server authentication.

#### Scenario: Launch installed application
- **WHEN** an installed dashboard opens without a valid session
- **THEN** its start URL requires login rather than exposing operational information

#### Scenario: Unsupported installation browser
- **WHEN** the browser does not provide an installation prompt
- **THEN** the dashboard continues to work as a normal website without a broken install control

### Requirement: Private-data-safe offline dashboard
A service worker scoped to /dashboard/ SHALL persist only an explicit allowlist of public static assets. Authenticated page HTML, API responses, credentials, worker APIs, mutations and media SHALL NOT be cached or replayed. Offline navigation SHALL show a public reconnection page without operational data; API failures SHALL remain failures. Online authorization failures SHALL not be replaced with cached content.

#### Scenario: Offline after viewing jobs
- **WHEN** a previously signed-in operator opens the installed dashboard offline
- **THEN** it shows the reconnection page without stored job, worker or video data

#### Scenario: Logout while service worker controls the page
- **WHEN** an operator logs out and reopens the dashboard online
- **THEN** the frontend still redirects to login using server session validation and API requests return unauthorized

### Requirement: Content-versioned PWA updates
The service worker and its public cache SHALL update with embedded content changes, remove only obsolete dashboard caches and preserve other applications' caches. An updated worker SHALL not force navigation or interrupt editing, intake or playback. The UI SHALL offer an explicit reload when an update is ready.

#### Scenario: New server version
- **WHEN** changed embedded assets produce a new worker/cache version
- **THEN** the updated public assets activate and a user-controlled reload is offered without automatic page replacement

### Requirement: English operations experience
The operations dashboard SHALL use English for application-provided navigation, labels, charts, controls, login, media exploration and playback, errors, accessibility text and PWA/offline content. Pages and the manifest SHALL declare English, and application-formatted numbers and dates SHALL use an English locale. User-provided names, paths and external diagnostics SHALL remain unchanged.

#### Scenario: Navigate and operate the dashboard
- **WHEN** an operator signs in, monitors jobs, changes capacity or browses and queues media
- **THEN** application-provided labels, feedback and media eligibility or error messages are in English

#### Scenario: Installed or offline dashboard
- **WHEN** an operator installs the dashboard or opens it without a connection
- **THEN** installation metadata, reconnection content and update controls are in English

#### Scenario: User media names
- **WHEN** a filename or worker name contains another language
- **THEN** the UI preserves that value while presenting surrounding application text in English

### Requirement: Independent frontend runtime
The frontend SHALL run separately with API network access only, serve all fleet HTML/assets/PWA content and proxy only dashboard API routes. The server SHALL serve no frontend pages/assets. Authentication, media filesystem access, probing, preview conversion, queue operations and telemetry SHALL stay on the server. The frontend SHALL require no media mounts, database or encoding tools.

#### Scenario: Frontend on another host
- **WHEN** the frontend has only network access to the configured TranscoderD API
- **THEN** login, statistics, worker controls, library exploration, reviewed intake and playback work without filesystem access on the frontend

#### Scenario: Seek and close playback
- **WHEN** an authenticated operator seeks original media or disconnects compatible playback through the frontend
- **THEN** byte-range responses are preserved and cancellation reaches server preview work without storing or buffering the complete video on the frontend

#### Scenario: API unavailable
- **WHEN** the server cannot validate a session or serve an API request
- **THEN** the frontend returns an explicit unavailable response without private HTML, fabricated data or cached API results

#### Scenario: Proxy boundary
- **WHEN** a client requests worker APIs, arbitrary paths, another upstream or a cross-origin mutation
- **THEN** the frontend rejects the request without forwarding it; an upstream redirect does not escape the configured API origin

### Requirement: Worker hardware summary and drill-down
Overview worker rows SHALL show concise measured CPU, GPU and encoder usage and link to that worker in the Workers view. Details SHALL retain worker controls and show available CPU/RAM history, per-device GPU/encoder history, VRAM, temperature, power, identities and sample freshness. Host/device scope and shared usage SHALL be explicit; encoder slots SHALL remain distinct from hardware utilization.

#### Scenario: Open worker from overview
- **WHEN** an operator clicks or keyboard-activates a worker summary
- **THEN** the Workers view selects that exact worker, shows its resource details and offers a route back to all workers

#### Scenario: Missing or stale sample
- **WHEN** measurements are missing, invalid or more than twenty seconds old, or the worker is offline
- **THEN** the UI identifies unavailable or last-reported readings instead of presenting zero or stale data as live usage

#### Scenario: Bounded resource history
- **WHEN** dashboard polling receives distinct resource samples
- **THEN** charts retain at most 120 samples/ten minutes in page memory, ignore duplicates, preserve gaps for missing data and write no private resource history to persistent browser storage
