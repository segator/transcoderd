## MODIFIED Requirements

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

## ADDED Requirements

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
