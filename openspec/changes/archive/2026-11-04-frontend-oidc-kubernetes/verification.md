# Verification — 2026-11-04

## Authentication and compatibility

- CLI/YAML/environment tests verify auth mode, private API credential, encryption key, issuer, client credentials, scopes/groups and CLI precedence. Server `TR_WEB_DASHBOARD_TOKEN` decode/CLI precedence is covered independently.
- Signed local OIDC fixtures exercise actual discovery, JWKS and code/token HTTP exchanges with ES256 signatures and PKCE verification. Rejections cover wrong issuer/audience/nonce/authorized party, missing multi-audience authorized party, expired/future-issued or tampered tokens, missing/wrong groups, state/cookie expiry, missing transaction cookie, incorrect PKCE and replayed authorization codes. Invalid configuration, short/Unicode credentials, invalid keys, insecure endpoints and failed discovery fail closed.
- Two frontend instances complete each other's login transactions and accept the encrypted session across requests. Tests reject browser bearer/legacy-cookie/token-login bypass, tampered/expired session cookies and old keys. Logout and API-session revocation deny replay on the other instance. An existing session remains usable while the provider is unavailable.
- `server/web/TestOIDCFrontendUsesRevocableAPISessions` uses the actual API auth, media roots, dashboard stats and worker-settings handlers: verified OIDC creates a revocable API session, settings writes succeed, logout invalidates both replicas and the dashboard service credential cannot authorize worker routes.
- Same-origin CSRF checks, media 206 ranges and private PWA cache restrictions remain covered. OIDC auth routes explicitly bypass service-worker interception.

## Checks

- Full Linux `go test -race ./...` passes, with focused final frontend/cmd/server-web race checks after validation hardening.
- Linux and Windows golangci-lint v2.11.4 report 0 issues.
- The frontend/web test executable runs natively on Windows through WSL interop: PASS, including the signed OIDC flows and compatibility tests.
- All fourteen JavaScript PWA/resource/activity tests pass.
- `mage docker:frontend` builds/loads the standalone frontend image with the pinned Dagger toolchain. The frontend runtime remains independent of backend/database/media packages.
- Both token and OIDC overlays render through `kubectl kustomize`; kubeconform v0.8.0 validates all ten rendered Kubernetes resources with strict schemas, zero invalid/error/skipped resources. No cluster was contacted or changed.
- Strict OpenSpec validation passes before synchronization and is repeated for the resulting main specs/archive.

## Real browser/container verification

Chromium uses the built frontend image in two separate nonroot UID/GID 65532 containers, each with a read-only rootfs, dropped capabilities and no mounts. An actual Traefik v3.7.17 container terminates HTTPS and balances both frontend instances. A separate API fixture uses real TranscoderD HTTP handlers and in-memory synthetic monitoring/media data; a separate local provider signs OIDC identities and enforces one-time codes/PKCE.

With the PWA already controlling the login page, the browser navigates across sites to the provider and returns to the fixed HTTPS callback. Verification covers SSO-only English login, desktop/mobile layouts, encrypted HttpOnly Secure SameSite=Lax session cookies, no raw API/provider credentials, repeated API requests and reloads through both replicas, remote CPU/GPU settings, original video byte ranges, public-only cache contents, logout and rejected revoked-cookie replay. The frontend containers and temporary provider/API/Traefik fixture are removed after the check.

The TLS certificate in this local fixture is Traefik's test/default certificate and Chromium explicitly accepts it for this test; ACME issuance and a real Authentik/Google tenant are not exercised. Deployment guidance is grounded in official Authentik provider/scope/Google-source and Traefik Ingress documentation. Tenant credentials, DNS and an existing Traefik Let's Encrypt resolver must be supplied by the operator.

The main local preview remains available at `http://localhost:18080/dashboard/` in compatible token mode (`encoding123`), talking to the independent API on `:18081`. It includes the prior worker-resource monitoring. No production service, worker workload, media job or Kubernetes deployment is modified.

Prior head `9186052` completed Linux/E2E, native Windows unit/build/end-to-end, PWA, lint and CodeQL checks successfully. The OIDC commit requires a fresh CI run after publication; prior results do not validate it. Images are published by the existing pipeline on main, not by PR builds.
