# Spec Delta

## ADDED Requirements

### Requirement: Installable operations dashboard
The server SHALL expose an embedded application manifest with stable dashboard identity, dashboard-scoped standalone launch, branded 192px/512px and maskable icons, theme metadata and Apple touch icon. Login and dashboard pages SHALL link installation metadata. Where supported, installation controls SHALL follow browser availability and disappear after installation. Installation SHALL retain normal server authentication.

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
- **THEN** server authentication still redirects to login and API requests return unauthorized

### Requirement: Content-versioned PWA updates
The service worker and its public cache SHALL update with embedded content changes, remove only obsolete dashboard caches and preserve other applications' caches. An updated worker SHALL not force navigation or interrupt editing, intake or playback. The UI SHALL offer an explicit reload when an update is ready.

#### Scenario: New server version
- **WHEN** changed embedded assets produce a new worker/cache version
- **THEN** the updated public assets activate and a user-controlled reload is offered without automatic page replacement
