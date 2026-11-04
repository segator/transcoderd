# Tasks

## 1. Configuration and authentication
- [x] 1.1 Add validated auth mode/secret/OIDC CLI, YAML and explicit environment bindings plus server dashboard-token environment binding; cover precedence, invalid configuration and compatibility.
- [x] 1.2 Implement discovery, state/nonce/PKCE, signed identity/group verification, encrypted replica-compatible cookies and API-owned sessions; prevent token/cookie fallback and preserve logout, CSRF, proxy streaming and PWA privacy with protocol/integration tests.

## 2. Deployment and operator experience
- [x] 2.1 Add English OIDC login UI, Authentik/Google configuration guide and runnable k3s ConfigMap/Secret/Deployment/Service/Traefik TLS examples for both modes, documenting secret handling, session lifetime and replica boundaries.

## 3. Verification and delivery
- [x] 3.1 Run Go race tests, lint, native Windows authentication tests, PWA and browser checks, validate Kubernetes manifests and strict OpenSpec; refresh the local independent preview and record actual evidence.

## Workflow follow-up
- Sync the delivered specs, archive the completed change and update PR #202 with fresh CI status.
