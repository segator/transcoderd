## MODIFIED Requirements

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

### Requirement: Protected dashboard mutations
Dashboard mutations SHALL require authentication and same-origin request protection. Responses SHALL prevent operational data caching. Reusable credentials SHALL NOT be included in page content or request URLs.

#### Scenario: Cross-origin mutation
- **WHEN** a request attempts login, logout, or a settings change from another origin
- **THEN** the server rejects the request without changing settings

## ADDED Requirements

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
