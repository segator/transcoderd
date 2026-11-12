# Operations Dashboard Delta

## ADDED Requirements

### Requirement: Fleet dashboard appearance
The fleet dashboard SHALL provide Day and Night themes and remember the selected theme in the browser. Night SHALL remain the first-visit default. The saved theme SHALL also apply to fleet login and offline pages.

#### Scenario: Return with Day selected
- **WHEN** an operator selects Day, reloads the page or returns to the login screen
- **THEN** the fleet interface continues to use Day and the operator can switch back to Night

## MODIFIED Requirements

### Requirement: Worker hardware summary and drill-down
Overview worker rows SHALL show measured CPU, GPU and encoder usage and link to that worker in the Workers view. Details SHALL show available CPU/RAM history, per-device GPU/encoder history, VRAM, temperature, power, identities, sample freshness and worker controls. Host/device scope and shared usage SHALL be explicit. Encoder slots SHALL remain distinct from hardware utilization.

#### Scenario: Open worker from overview
- **WHEN** an operator clicks or keyboard-activates a worker summary
- **THEN** the Workers view selects that worker, shows its resource details and offers a route back to all workers
- **AND** the details for a combined Windows worker provide access to both profiles' resources

#### Scenario: Missing or stale sample
- **WHEN** measurements are missing, invalid or more than twenty seconds old, or the worker is offline
- **THEN** the UI identifies unavailable or last-reported readings instead of presenting zero or stale data as live usage

#### Scenario: Bounded resource history
- **WHEN** dashboard polling receives distinct resource samples
- **THEN** charts retain at most 120 samples/ten minutes in page memory, ignore duplicates, preserve gaps for missing data and write no private resource history to persistent browser storage

### Requirement: Replica-compatible OIDC session boundary
OIDC browser sessions SHALL encrypt the API session in a frontend-only HttpOnly cookie using a configured shared key, with Secure cookies for HTTPS. Provider tokens and API login credentials SHALL remain server-side. Verified OIDC dashboard sessions SHALL expire 15 days after login and survive restarts of both services with the same configuration and API database. Browser bearer headers and legacy API cookies SHALL NOT bypass OIDC. Sessions and login callbacks SHALL NOT enter PWA caches.

#### Scenario: Another frontend pod serves the next request
- **WHEN** frontend replicas share authentication configuration and a session encryption key
- **THEN** login state and valid sessions work across replicas without sticky sessions or frontend persistent storage

#### Scenario: Browser reopens after login
- **WHEN** an operator closes and reopens the browser before the dashboard session expires, including after the provider ID token expires or either TranscoderD service restarts
- **THEN** the valid OIDC dashboard session continues to grant access

#### Scenario: Logout or backend session revocation
- **WHEN** an operator logs out successfully or the API session expires
- **THEN** replaying the old frontend cookie does not authorize dashboard data

#### Scenario: Attempted token bypass
- **WHEN** OIDC mode receives an API token bearer header, legacy dashboard cookie or token-login request without a valid OIDC session
- **THEN** it rejects access and never exposes the private API login credential

#### Scenario: Frontend upgraded before API
- **WHEN** a verified OIDC login creates a session through an older API
- **THEN** the browser session follows the expiry returned by that API

### Requirement: Fixed worker profile controls
The fleet dashboard SHALL present matched CPU and GPU profiles of the same Windows service as one worker in Overview and Workers, with shared host scope. Each profile SHALL expose its supported concurrency, commands and relevant resource readings. Independent workers SHALL retain both CPU and GPU controls. A profile's capacity update SHALL save its own pair atomically with zero for its unsupported class.

#### Scenario: Matched Windows profiles
- **WHEN** a managed CPU profile and GPU profile report matching service names and host identity
- **THEN** they count as one fleet worker and share one card with distinct CPU and GPU sections and attributed work
- **AND** saving one profile's concurrency keeps the other profile's value and control state

#### Scenario: Unmatched worker identities
- **WHEN** independent workers use similar names or two profiles cannot be associated with the same host
- **THEN** the dashboard continues to display them separately

#### Scenario: Windows CPU profile
- **WHEN** an operator opens a managed CPU profile
- **THEN** its section shows CPU controls and CPU/RAM metrics

#### Scenario: Windows GPU profile
- **WHEN** an operator opens a managed GPU profile
- **THEN** its section shows GPU controls and GPU/RAM metrics

### Requirement: Concise responsive operations layout
Navigation, login and section headings SHALL use functional names without promotional slogans. Overview SHALL group selected-period measurements separately from live/all-time measurements and identify the period scope. Mobile layouts SHALL use multiple columns for compact metrics and small resource/analytics charts when viewport width permits, while keeping task controls readable and avoiding page-level horizontal overflow.

#### Scenario: Mobile monitoring
- **WHEN** an operator opens Overview or worker resource details at a typical mobile width
- **THEN** compact metrics and compatible small charts share rows, larger task/control sections retain usable widths, and the page has no horizontal overflow

#### Scenario: Change concurrency on a phone
- **WHEN** an operator opens an online worker card at a typical mobile width
- **THEN** the worker's concurrency control is labeled Concurrent encodes and its compact numeric input and Save button share a row
