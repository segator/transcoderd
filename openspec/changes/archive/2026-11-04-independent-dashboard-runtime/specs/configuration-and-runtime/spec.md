## ADDED Requirements

### Requirement: Independent frontend configuration and lifecycle
The frontend SHALL accept CLI flags, YAML and explicit TR_FRONTEND_LISTEN, TR_FRONTEND_API_URL and TR_FRONTEND_PUBLIC_URL environment bindings, with CLI precedence. Defaults SHALL be frontend.listen=:8081, frontend.apiURL=http://localhost:8080 and frontend.publicURL empty (direct request origin). API/public URLs SHALL be HTTP(S) origins without credentials, query or fragments. Shutdown SHALL stop HTTP service and cancel remaining streams.

#### Scenario: Remote API and external HTTPS origin
- **WHEN** the operator configures a remote API URL and HTTPS public URL
- **THEN** the frontend listens independently, sends backend requests only to that API origin, validates browser mutations against the public origin and marks session cookies Secure

#### Scenario: Invalid origin configuration
- **WHEN** an API/public URL includes credentials, a non-root path, an unsupported scheme, query or fragment
- **THEN** frontend startup fails before accepting requests

#### Scenario: Frontend without backend storage
- **WHEN** the frontend starts with only its runtime configuration and embedded assets
- **THEN** it starts without initializing PostgreSQL, opening media folders or locating FFmpeg
