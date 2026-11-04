# Proposal

## Why
The independent dashboard will run on k3s behind Traefik TLS termination and needs configurable authentication suitable for Authentik federated with Google, while retaining single-user token login.

## What Changes
- Select frontend authentication mode: token (compatible default) or OIDC with discovery and authorization-code login.
- Keep OIDC secrets and a private API login credential on the frontend runtime. Exchange successful identity verification for an API-owned revocable session, encrypted in a frontend cookie shared across replicas.
- Require validated signatures/issuer/audience/expiry, nonce/state and PKCE, optional group restriction, fixed public HTTPS callback and same-origin operations; OIDC mode rejects token-login fallback.
- Add explicit environment bindings and Kubernetes Secret/Deployment/Service/Traefik Ingress examples, with documented Authentik/Google setup and token-mode configuration.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `operations-dashboard`: selectable browser authentication and OIDC trust/session boundaries.
- `configuration-and-runtime`: authentication configuration, strict startup validation and deployment settings.

## Impact
Frontend auth/proxy/configuration, Go OIDC/OAuth dependencies, server dashboard-token environment binding, authentication regression/integration tests and deployment/operator references. No frontend media/storage/database dependency or production deployment.
