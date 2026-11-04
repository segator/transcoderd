## ADDED Requirements

### Requirement: Frontend authentication configuration
The frontend SHALL accept CLI/YAML and explicit environment settings for auth.mode (default token), apiToken and auth.sessionKey (empty), and OIDC issuerURL, clientID, clientSecret, scopes (openid, profile, email), allowedGroups (empty). OIDC SHALL require a fixed public URL, private API credential of at least 32 characters, base64 32-byte key and complete provider settings. Unknown modes and invalid/incomplete settings SHALL fail startup.

#### Scenario: Kubernetes configuration and TLS termination
- **WHEN** a frontend pod receives its API/public URLs via configuration and OIDC credentials/key/API token via Kubernetes Secret environment variables
- **THEN** it runs without media mounts or a database, uses its configured HTTPS public origin for callbacks and cookies, and ignores untrusted forwarded origins

#### Scenario: Default token deployment
- **WHEN** auth.mode is token or omitted
- **THEN** the compatible single-user API token login works without OIDC configuration or a frontend session key

#### Scenario: Invalid provider or secret configuration
- **WHEN** OIDC settings are incomplete, origins/endpoints use insecure non-loopback HTTP, the session key is invalid or discovery fails
- **THEN** the frontend refuses to start without downgrading to token authentication

### Requirement: Explicit dashboard token environment
The server SHALL bind TR_WEB_DASHBOARD_TOKEN to web.dashboardToken with normal CLI precedence, allowing its dashboard login credential to be supplied through a runtime secret without changing worker authentication.

#### Scenario: Runtime secret for API login
- **WHEN** the API receives TR_WEB_DASHBOARD_TOKEN and OIDC frontend.apiToken has the same value
- **THEN** verified OIDC login can create an API session while worker routes still require their independent web.token
