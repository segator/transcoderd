# operations-dashboard Specification

## Purpose

Provide authenticated fleet monitoring and individual worker capacity controls using measured job outcomes and live worker progress.

## Requirements

### Requirement: Dashboard authentication
The independent frontend SHALL select token (default) or OIDC browser login. Token mode SHALL use server web.dashboardToken (default encoding123); OIDC mode SHALL require verified identity and reject token fallback. The server SHALL validate API-owned expiring sessions at GET /dashboard/api/session (204/401). Sessions SHALL use HttpOnly SameSite cookies and logout SHALL revoke them. Dashboard credentials SHALL NOT authorize worker routes.

#### Scenario: Anonymous access
- **WHEN** an unauthenticated visitor opens the dashboard or requests its data
- **THEN** the frontend page redirects to login and server data/session requests return HTTP 401 without operational data

#### Scenario: Valid login and logout
- **WHEN** a visitor supplies the configured dashboard token in token mode or completes verified OIDC login
- **THEN** the server creates a session that permits dashboard access until expiry or browser logout

#### Scenario: Invalid credentials
- **WHEN** a visitor supplies an incorrect or empty token in token mode
- **THEN** login returns HTTP 401 without creating a session

### Requirement: Protected dashboard mutations
Dashboard mutations SHALL require authentication and same-origin request protection. Responses SHALL prevent operational data caching. Reusable credentials SHALL NOT be included in page content or request URLs.

#### Scenario: Cross-origin mutation
- **WHEN** a request attempts login, logout, or a settings change from another origin
- **THEN** the server rejects the request without changing settings

### Requirement: Useful fleet measurements
The dashboard SHALL show current queued, active, completed, failed, and canceled job counts; finalized input/output sizes and weighted storage savings; completion and failure history; worker statistics; and bounded recent jobs and activity. Period filters SHALL support 24 hours, 7 days, 30 days and custom date ranges. Terminal job counts SHALL use Job-level outcomes, not intermediate phase completion.

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
The dashboard SHALL offer authenticated folder navigation, selectable videos, editable presets, an embedded player and reviewed submission. Job rows SHALL label playback as queued source or completed output; active, failed and canceled jobs SHALL have no playback action. Scan/load progress, actual successes, skipped/failed reasons, cancellation, empty/error states and unavailable files SHALL be shown without invented data.

#### Scenario: Browse and play queued source
- **WHEN** an operator opens a queued job's available source
- **THEN** its video can be played inside the dashboard without worker assignment or a worker credential

#### Scenario: Active or failed job
- **WHEN** a job is active, failed or canceled
- **THEN** its row has no playback action and its job-assets endpoint rejects playback

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
The frontend SHALL run separately with network access to the API and, in OIDC mode, its identity provider, serve all fleet HTML/assets/PWA content and proxy only dashboard API routes. The server SHALL serve no frontend pages/assets. API session validation, media filesystem access, probing, preview conversion, queue operations and telemetry SHALL stay on the server; OIDC browser login SHALL run on the frontend. The frontend SHALL require no media mounts, database or encoding tools.

#### Scenario: Frontend on another host
- **WHEN** the frontend uses token mode and has only network access to the configured TranscoderD API
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

### Requirement: Verified OIDC browser login
OIDC login SHALL use discovery, authorization code and PKCE S256 with a browser-bound expiring state and nonce. Signed ID tokens SHALL validate issuer, audience, expiry and nonce before any API session is created. Optional allowedGroups SHALL require a matching ID-token groups claim. Redirects SHALL use the configured public origin and fixed callback path; provider failures SHALL fail closed without exposing secrets.

#### Scenario: Authentik with upstream Google
- **WHEN** Authentik completes authentication, including through a configured Google source, and returns a valid code and matching identity
- **THEN** the frontend creates an API-owned session and opens the dashboard without exposing the API credential or provider tokens

#### Scenario: Invalid or replayed callback
- **WHEN** state, nonce, PKCE, signature, issuer, audience, expiry or authorization code validation fails, or the login cookie is missing or expired
- **THEN** login fails without an authenticated browser session

#### Scenario: Group restriction
- **WHEN** allowedGroups is configured and the verified ID token has no matching group
- **THEN** login returns forbidden without opening an API session

### Requirement: Replica-compatible OIDC session boundary
OIDC browser sessions SHALL encrypt the API session in a frontend-only HttpOnly cookie using a configured shared key, with Secure cookies for HTTPS. Provider tokens and API login credentials SHALL remain server-side. API session expiry, logout and API restart SHALL revoke access across frontend replicas; browser bearer headers and legacy API cookies SHALL NOT bypass OIDC. Sessions and login callbacks SHALL NOT enter PWA caches.

#### Scenario: Another frontend pod serves the next request
- **WHEN** frontend replicas share authentication configuration and a session encryption key
- **THEN** login state and valid sessions work across replicas without sticky sessions or frontend persistent storage

#### Scenario: Logout or backend session revocation
- **WHEN** an operator logs out successfully, the API session expires or the API restarts
- **THEN** replaying the old frontend cookie does not authorize dashboard data

#### Scenario: Attempted token bypass
- **WHEN** OIDC mode receives an API token bearer header, legacy dashboard cookie or token-login request without a valid OIDC session
- **THEN** it rejects access and never exposes the private API login credential

### Requirement: Filtered paginated jobs
The dashboard SHALL query all jobs by state and case-insensitive filename, worker or ID search before pagination. Pages SHALL contain at most 100 jobs with a matching total and navigation; the default page size SHALL be 25. Overview counters SHALL remain global.

#### Scenario: Completed jobs behind a large queue
- **WHEN** more than 100 queued jobs precede completed jobs
- **THEN** selecting Completed returns completed jobs with their matching total and further pages remain accessible

### Requirement: Recent fleet membership and offline presentation
Fleet counts and worker listings SHALL include only workers seen within the previous 30 days, independently of the selected statistics period. Online workers SHALL appear first, with active workers before idle peers. Worker lists SHALL paginate at 12 per page. Offline workers SHALL show basic identity and last-seen information without capacity controls or live resource details.

#### Scenario: Historical worker
- **WHEN** a worker has no heartbeat or was last seen over 30 days ago
- **THEN** it does not contribute to the fleet total or worker list

#### Scenario: Offline recent worker
- **WHEN** a worker was seen within 30 days but is currently offline
- **THEN** it appears after online workers as Offline without editable limits

### Requirement: Fixed worker profile controls
Fixed Windows CPU and GPU worker profiles SHALL identify their profile and shared host scope. Each SHALL expose only its supported encoder limit and relevant CPU or GPU utilization; independent workers SHALL retain both limits. Limits SHALL still be saved atomically with the unsupported class set to zero.

#### Scenario: Windows CPU profile
- **WHEN** an operator opens a managed CPU profile
- **THEN** CPU controls and CPU/RAM metrics are shown without GPU controls or GPU utilization

#### Scenario: Windows GPU profile
- **WHEN** an operator opens a managed GPU profile
- **THEN** GPU controls and GPU/RAM metrics are shown without CPU controls or CPU utilization

### Requirement: Signed-in operator identity
OIDC sessions SHALL retain verified name, preferred username, email and subject display claims inside the encrypted frontend cookie. An authenticated, non-cacheable identity response SHALL expose display identity without credentials or provider tokens. The dashboard SHALL display a name or username and email when available; token login SHALL identify a token session without inventing a user.

#### Scenario: Verified account
- **WHEN** a verified OIDC identity includes a name and email
- **THEN** the dashboard displays that name and email across frontend replicas

#### Scenario: Revoked session
- **WHEN** the API session has been revoked
- **THEN** the identity endpoint returns unauthorized even if the frontend cookie decrypts

### Requirement: Explicit acknowledged remote worker controls
Online capable workers SHALL offer Pause in RAM/Resume encodes separately from Stop after current/Resume queue. Commands SHALL require authenticated same-origin requests and worker acknowledgement, with pending/applied/failed feedback. Queue resume SHALL restore the last positive limits. RAM resume SHALL preserve queue and individual holds. Unsupported/offline/maintenance workers SHALL not accept these commands.

#### Scenario: Pause a running encode remotely
- **WHEN** an operator selects Pause in RAM
- **THEN** the worker suspends owned encodes through its local RAM control, retains limits and memory, acknowledges application, and the UI offers Resume encodes

#### Scenario: Stop and resume queue
- **WHEN** an operator selects Stop after current then Resume queue
- **THEN** active encodes finish while queued starts and admission stop, then the previous positive limits are restored without clearing independent RAM or individual holds

#### Scenario: Failed or superseded command
- **WHEN** a control fails or a newer command supersedes an unacknowledged command
- **THEN** the current command reports failure or pending accurately and an old acknowledgement cannot complete the newer command

### Requirement: Reporting-period worker visibility
The Workers view SHALL include only workers seen within its selected 24-hour, 7-day, 30-day or custom reporting period. Fleet-wide membership SHALL retain its 30-day maximum; custom-period visibility SHALL filter last-seen timestamps by both boundaries within that set, without claiming historical resource readings. Filtered worker counts and pagination SHALL describe the visible set.

#### Scenario: Last 24 hours
- **WHEN** Last 24 hours is selected and a worker was last seen two days ago
- **THEN** it is absent from Workers while a worker seen within 24 hours remains visible

#### Scenario: Custom range worker visibility
- **WHEN** a custom period ends before a worker's last-seen timestamp
- **THEN** that worker is absent from the period-filtered Workers list without altering live fleet totals

### Requirement: Compact worker resource history
Online worker cards SHALL show small graphs for available CPU, RAM, GPU and encoder usage according to their supported profile, using bounded samples received in the current tab. Missing/stale readings SHALL remain explicit and gaps SHALL not imply measured usage. Graphs SHALL not require opening worker details.

#### Scenario: Profile card history
- **WHEN** distinct hardware samples arrive for a CPU or GPU Windows profile
- **THEN** its card shows RAM history and only its relevant CPU or GPU/encoder history

### Requirement: Reporting-range boundaries and scope
Custom ranges SHALL use start-inclusive/end-exclusive RFC3339 boundaries, spanning at most 3660 days. Invalid, incomplete, reversed or mixed preset/range requests SHALL be rejected. Period summaries, timelines, codec/worker outcomes and activity SHALL use both boundaries. Measured period source/output/saved bytes SHALL be separate from all-time totals; live jobs and fleet presence SHALL remain independent of outcome filters with explicit scope labels.

#### Scenario: Historical range savings
- **WHEN** an operator selects a past date range
- **THEN** completed/failed counts and finalized savings include only outcomes at or after its start and before its end, while live and all-time values remain separately labeled

#### Scenario: Invalid reporting range
- **WHEN** a range has a missing, invalid or reversed boundary, exceeds 3660 days or is combined with hours
- **THEN** it returns HTTP 400 without running a dashboard query

#### Scenario: API without custom-range support
- **WHEN** a custom-range response omits its boundaries or returns boundaries different from the request
- **THEN** the frontend shows an API upgrade error and retains the previous results with their previous period label

### Requirement: Individual remote encode control
Online worker cards SHALL show each current task's filename, phase, progress and pause state. Capable workers SHALL offer authenticated same-origin acknowledged task Pause/Resume. Commands SHALL validate UUID and fresh controllable ownership at submission and application, preserve unrelated holds, and report pending/failure. Older workers SHALL retain worker-wide controls without receiving individual commands.

#### Scenario: Pause one encode
- **WHEN** an operator pauses one of two active encodes
- **THEN** only the selected encode suspends, its progress is retained, and its Resume action appears after acknowledgement

#### Scenario: Resume under worker RAM hold
- **WHEN** an operator resumes an individually paused encode while the worker RAM hold remains active
- **THEN** only the individual hold clears and the encode stays suspended by the worker

#### Scenario: Ownership changes before application
- **WHEN** a queued command targets a job that finishes or leaves the worker before application
- **THEN** it fails explicitly without affecting another encode


#### Scenario: Invalid or unsupported individual request
- **WHEN** a request lacks a valid job UUID, targets another worker's encode or targets a version-1 worker
- **THEN** it is rejected without queuing individual control or modifying any job

### Requirement: Concise responsive operations layout
Navigation, login and section headings SHALL use functional names without promotional slogans. Overview SHALL group selected-period measurements separately from live/all-time measurements and identify the period scope. Mobile layouts SHALL use multiple columns for compact metrics and small resource/analytics charts when viewport width permits, while keeping task controls readable and avoiding page-level horizontal overflow.

#### Scenario: Mobile monitoring
- **WHEN** an operator opens Overview or worker resource details at a typical mobile width
- **THEN** compact metrics and compatible small charts share rows, larger task/control sections retain usable widths, and the page has no horizontal overflow

### Requirement: Reported worker software version
Workers SHALL include the running binary version in runtime telemetry. Authenticated fleet cards, overview summaries and selected resource details SHALL display that reported version. Offline workers SHALL retain it as last-reported information. Missing versions SHALL show Version unavailable, without inference from protocol capabilities, server version or release tags. Heartbeats from older workers SHALL remain accepted.

#### Scenario: Worker release changes
- **WHEN** a worker reports a new running version in its heartbeat
- **THEN** the dashboard shows that version after its normal refresh without a manual page reload

#### Scenario: Offline worker
- **WHEN** a recently seen worker with a recorded version goes offline
- **THEN** its basic card shows the last-reported version without enabling live controls

#### Scenario: Older worker without version
- **WHEN** runtime telemetry omits the version or is absent
- **THEN** the dashboard shows Version unavailable and continues showing the supported worker information
