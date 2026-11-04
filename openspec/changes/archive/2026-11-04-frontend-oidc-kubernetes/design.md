# Design

## Context
The separate Go frontend proxies an allowlist of dashboard APIs. The API owns opaque, revocable twelve-hour sessions in memory. Existing token login and PWA privacy must remain compatible. The frontend image already runs nonroot without media/database/tool dependencies.

## Goals / Non-Goals
Support Authentik OIDC and single-user token modes, public TLS termination and multiple interchangeable frontend pods. No deployment to the user's cluster, per-user roles, Google client inside TranscoderD, provider-wide/back-channel logout or API high-availability session store is included. All authorized dashboard identities have existing operator permissions.

## Decisions
- Keep token mode unchanged and default. OIDC runs at the frontend rather than the media server because its public origin owns the browser callback. Use maintained coreos/go-oidc and x/oauth2 libraries for discovery, signature/claim validation and PKCE. Discovery and exchanges use bounded clients, normal TLS verification and no redirect following; permit plain HTTP only for loopback development endpoints.
- The fixed callback is frontend.publicURL + /dashboard/auth/callback. Encrypted ten-minute SameSite=Lax login cookies carry state/nonce/PKCE verifier; distinct encryption purposes prevent mixing transaction and session payloads. Successful callbacks validate all claims, optional groups and one-time provider code before contacting the API.
- frontend.apiToken is a strong server-side login credential matching API web.dashboardToken. It is used only to POST API login after identity verification, never as unconditional proxy Authorization. Encrypt the resulting opaque API session in a twelve-hour frontend cookie with AES-256-GCM and a shared base64 key. Per-request proxying sends only that backend session, so API restart/expiry/logout revokes access across frontend replicas without an extra session database. No provider tokens need to persist. Token-mode cookies cannot authorize OIDC sessions.
- OIDC token-login fallback is disabled. Preserve same-origin mutation protection, explicit route allowlist, no private caches and media range/cancellation behavior. Logout revokes the API session and expires frontend cookies; it leaves the Authentik/Google SSO session intact. Signing/encryption key rotation invalidates frontend cookies.
- Scopes default to openid/profile/email; openid is always requested. Optional allowedGroups uses the signed groups array. Empty group restriction delegates admission to Authentik application policies. Register a confidential Authentik client with an asymmetric signing key and strict callback; Google is an Authentik source, not an app-side second issuer.
- Document immutable image tags, ConfigMap/Secret env bindings, unprivileged read-only Deployment, Service and standard Traefik Ingress using an existing Let's Encrypt resolver. No shared volumes or sticky sessions. Multiple API replicas would need their own shared session store and are outside this change.

## Risks / Trade-offs
- Authentik configuration differs by tenant: document exact issuer/discovery mapping and strict callback, and verify protocol behavior with a local signed provider instead of claiming a live tenant test.
- Shared frontend encryption key is required across pods/restarts: store it in a Secret and document rotation. API session revocation remains centralized.
- Users authorized by the IdP have full operator permissions: expose optional groups and document Authentik policy bindings.
- Provider availability is required for startup/new login; current sessions continue through the API. No silent mode downgrade.

## Migration Plan
Token mode remains default. For OIDC, set a strong API dashboard token, create an Authentik application/client and Google source, configure frontend Secret values and HTTPS public URL, then deploy the frontend image behind Traefik. Existing token sessions do not become OIDC sessions. Roll back by explicitly selecting token mode and signing in with the API dashboard token. Keep API/worker routes on the private API origin.
