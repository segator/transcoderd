# Frontend authentication, Authentik and Kubernetes

The fleet UI is its own `transcoderd-frontend` Docker runtime. It can run on another server or a k3s cluster behind Traefik and Let's Encrypt. It needs network access to the API, and in OIDC mode to the identity provider, without media folders, PostgreSQL or FFmpeg. Frontend image publication now includes Linux amd64 and arm64 under the same tag. Select a released multi-platform version or index digest before scheduling ARM64 nodes; older published tags retain their original architecture. The Kubernetes example selects Linux without pinning one CPU architecture.

## Choose an authentication mode

`frontend.auth.mode` / `TR_FRONTEND_AUTH_MODE` accepts `token` (default) or `oidc`. Unknown modes fail startup. Every authenticated dashboard user has full operator permissions; there is no per-user RBAC in this change.

**Token:** configure the API's `web.dashboardToken` using YAML, `--web.dashboardToken` or `TR_WEB_DASHBOARD_TOKEN`. The operator enters that token at the frontend login page. The initial compatibility default remains `encoding123`. The frontend needs the API URL and public browser URL; it needs no copy of the token, session key or OIDC settings. The API creates/revokes twelve-hour opaque sessions. Worker `web.token` stays independent.

```yaml
frontend:
  listen: ":8081"
  apiURL: "http://transcoder-api.internal:8080"
  publicURL: "https://transcoder.example.com"
  auth:
    mode: token
```

**OIDC:** the browser signs in at Authentik. The frontend verifies the identity, then uses its private `frontend.apiToken` to create an ordinary API-owned session. The API credential must match `web.dashboardToken`, contain at least 32 characters, and remain private to the two runtimes. Do not use the worker API token. Token login and browser bearer/legacy-cookie fallback are disabled on the OIDC frontend.

```yaml
frontend:
  listen: ":8081"
  apiURL: "http://transcoder-api.internal:8080"
  publicURL: "https://transcoder.example.com"
  apiToken: "<private value matching API web.dashboardToken>"
  auth:
    mode: oidc
    sessionKey: "<base64-encoded 32 random bytes>"
    oidc:
      issuerURL: "https://auth.example.com/application/o/transcoderd/"
      clientID: "<Authentik client ID>"
      clientSecret: "<Authentik client secret>"
      scopes: [openid, profile, email]
      allowedGroups: [transcoderd-operators]
```

Prefer runtime secrets for `apiToken`, `sessionKey` and `clientSecret`. Generate separate values with `openssl rand -hex 32` for the API credential and `openssl rand -base64 32` for the encryption key. An existing strong API dashboard token can be reused as the private API login credential. All frontend pods must share the encryption key; rotating it invalidates their cookies. Do not regenerate it on each pod start. After changing Secret-backed environment values, restart/roll out all frontend pods to load them.

## Settings and environment variables

Each key below also has the equivalent `--frontend.<key>` CLI flag; CLI overrides environment and YAML. Use `--config /etc/transcoderd/frontend.yaml` for an explicit file.

| Frontend key | Environment | Default / meaning |
| --- | --- | --- |
| `listen` | `TR_FRONTEND_LISTEN` | `:8081` |
| `apiURL` | `TR_FRONTEND_API_URL` | `http://localhost:8080`; fixed API origin |
| `publicURL` | `TR_FRONTEND_PUBLIC_URL` | Empty for direct token mode; required external HTTPS origin for OIDC |
| `apiToken` | `TR_FRONTEND_API_TOKEN` | Empty; private API login credential required only for OIDC |
| `auth.mode` | `TR_FRONTEND_AUTH_MODE` | `token` or `oidc` |
| `auth.sessionKey` | `TR_FRONTEND_AUTH_SESSION_KEY` | Empty; standard base64 encoding of 32 random bytes, required for OIDC |
| `auth.oidc.issuerURL` | `TR_FRONTEND_AUTH_OIDC_ISSUER_URL` | Exact provider issuer, including application path/trailing slash |
| `auth.oidc.clientID` | `TR_FRONTEND_AUTH_OIDC_CLIENT_ID` | Confidential OIDC client ID |
| `auth.oidc.clientSecret` | `TR_FRONTEND_AUTH_OIDC_CLIENT_SECRET` | Confidential OIDC client secret |
| `auth.oidc.scopes` | `TR_FRONTEND_AUTH_OIDC_SCOPES` | `openid,profile,email`; comma-separated in env/CLI; openid is always included |
| `auth.oidc.allowedGroups` | `TR_FRONTEND_AUTH_OIDC_ALLOWED_GROUPS` | Empty; comma-separated group names, any match grants admission |

API/public origins have no credentials, non-root path, query or fragment. Issuer URLs include the provider path. OIDC requires HTTPS for public/provider endpoints, except plain HTTP loopback development. Certificates are verified normally. Missing secrets, failed discovery and invalid endpoint configuration fail startup without falling back to token mode. The frontend ignores caller-supplied forwarded host/protocol headers; use the actual `publicURL` for Traefik TLS termination.

## Configure Authentik

1. Create an **OAuth2/OpenID Provider** and associated application, for example slug `transcoderd`. Select a **confidential** client and copy its client ID/secret.
2. Use a signing certificate/key for asymmetric signed ID tokens. Leave token encryption disabled; the application validates signed ID tokens through discovery/JWKS.
3. Register exactly this redirect URI in **strict** matching mode: `https://transcoder.example.com/dashboard/auth/callback`. The authorization code flow uses PKCE S256, state and nonce automatically.
4. Use **per-provider issuer** mode. Set `issuerURL` to the exact issuer shown by the provider, typically `https://auth.example.com/application/o/transcoderd/`. Its discovery URL is that issuer plus `.well-known/openid-configuration`. Global issuer mode has a different discovery location and is not inferred from a root issuer by this client.
5. Enable the default `openid`, `profile` and `email` scope mappings and **Include claims in id_token**. Authentik's default profile mapping supplies group names in the signed `groups` array. If `allowedGroups` is configured, make sure the intended group is present there; absent/nonmatching groups deny login. An empty allowlist delegates access to your Authentik application policy bindings.
6. Bind the intended users/groups/policies to the Authentik application. The example overlay additionally requires membership in `transcoderd-operators`; adjust that name to your group.
7. Set the application's launch URL to `https://transcoder.example.com/dashboard/`.

For Google federation, configure a **Google OAuth source in Authentik**, add it to the Authentik login flow and configure Google's consent/client settings there. Google's redirect points to Authentik, such as `https://auth.example.com/source/oauth/callback/google/`, rather than the TranscoderD callback. TranscoderD trusts Authentik as its single issuer and needs only the Authentik client credentials. Follow the official [Google source guide](https://docs.goauthentik.io/users-sources/sources/social-logins/google/cloud/), [OIDC provider guide](https://docs.goauthentik.io/add-secure-apps/providers/oauth2/) and [scope mapping reference](https://docs.goauthentik.io/add-secure-apps/providers/property-mappings/).

## Deploy on k3s with Traefik

Examples are under `deploy/kubernetes/frontend/`:

- `token/`: ConfigMap, two-replica Deployment, Service, Namespace and HTTPS Ingress; no frontend credentials required.
- `oidc/`: the same deployment with OIDC configuration and three `secretKeyRef` values. `secret.example.yaml` is deliberately excluded from the overlay; populate an actual Secret with your secret manager.

Before applying, edit the base API/public URLs, Ingress host/TLS host and the `letsencrypt` resolver annotation to match your cluster. The resolver must already be configured in Traefik with your ACME contact/challenge/storage settings. Set `websecure` / `ingressClassName` to the installed Traefik names. These are standard Kubernetes Ingress resources with [Traefik TLS resolver annotations](https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/ingress/); no ForwardAuth middleware or Authentik outpost is required. Existing cluster HTTP-to-HTTPS redirection can remain at Traefik.

The `kustomize edit` command requires the standalone Kustomize CLI; alternatively add an `images` override to the overlay. Select a released frontend image version or immutable digest containing this feature before deployment. The example's `frontend-main` is a moving tag and contains the feature only after it lands on main. To override the image in your chosen overlay:

```bash
cd deploy/kubernetes/frontend/oidc  # or ../token
kustomize edit set image "ghcr.io/segator/transcoderd=ghcr.io/segator/transcoderd:frontend-<version>"
kubectl kustomize .
```

For OIDC, create the namespace and `transcoderd-frontend-auth` Secret before starting the pods. Its keys are `api-token`, `session-key` and `client-secret`, matching `secret.example.yaml`. The API must receive the same `api-token` value as `TR_WEB_DASHBOARD_TOKEN`. Change the overlay's issuer/client ID/group settings to your Authentik provider. Preview and apply the selected overlay when ready:

```bash
kubectl apply -k deploy/kubernetes/frontend/oidc  # or token
kubectl -n transcoderd-ui rollout status deployment/transcoderd-frontend
```

The pods run as UID/GID 65532 with a read-only root filesystem, dropped capabilities, no service-account token and no volumes. `/healthz` probes check the frontend process, independently of API/provider availability. Configure any cluster egress policy to allow DNS, the API and the issuer/JWKS/token endpoints. The browser must reach the frontend and Authentik. Source/output files and media tools remain solely on the API server; video ranges and cancellation stream through the frontend.

## Session behavior and operations

OIDC uses an encrypted HttpOnly SameSite=Lax frontend cookie (Secure with HTTPS), a separate ten-minute browser-bound login cookie and a fixed callback path. The API session inside it stays encrypted. State/nonce, PKCE, signature, issuer, audience, authorized party and expiry are checked; provider access/ID/refresh tokens are not stored in browser cookies or persistent storage. The app session lasts up to twelve hours, bounded by its API session, independently of subsequent provider-token expiry. Existing operator pages remain English and installable as a PWA; OIDC routes bypass service-worker interception and private data is never cached.

Successful **Sign out** revokes the API session and removes the frontend cookie across all frontend replicas. API restart or session expiry also revokes access. It signs out of TranscoderD only; the Authentik/Google session remains, so the next SSO login may be immediate. Provider-wide/back-channel logout and continuous rechecking of group changes are not implemented. To revoke existing sessions immediately, revoke/restart API sessions or rotate the frontend session key; changing the issuer/client/public origin also invalidates encrypted cookies.

Multiple frontend replicas need the same configuration/key and the same API instance; no sticky sessions or frontend storage are required. The current API session store is in-memory and does not support arbitrary load balancing across multiple independent API replicas. Expose the frontend to browsers and keep the separate API origin private to workers/frontend; its dashboard token remains a privileged service credential in OIDC mode.

## Signed-in identity

The account area displays the verified OIDC name (falling back to preferred username, email or subject) and email/username; mobile layouts show the account in the top bar. Verified display claims are bounded and retained inside the encrypted frontend session, shared across replicas. `GET /dashboard/api/identity` is frontend-owned, validates the API session on every request and returns non-cacheable display fields without credentials/provider tokens. Token mode identifies **Token session** and **Dashboard token**, without fabricating a username.

Sessions created before this change contain no display claims. Sign out and sign in once after upgrading to populate the account display. Authentik must include the profile/email claims in the signed ID token using the scope mappings above.
