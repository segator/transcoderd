# Design

## Context
The current server owns embedded dashboard files alongside authenticated dashboard/media APIs. The frontend host will have API network access only. The worker-local desktop dashboard remains a distinct local administration UI.

## Goals / Non-Goals
Provide a genuinely independent frontend executable/image requiring neither backend packages nor media mounts, PostgreSQL, FFmpeg or worker credentials. Retain existing browser URLs, English copy, playback/seek behavior, authentication and PWA privacy. Do not change worker protocols or server-side media processing.

## Decisions
- Move all fleet UI assets/PWA handling into frontend/web, linked only into transcoderd-frontend. The server exposes API routes only and a protected GET /dashboard/api/session returning 204/401.
- The frontend proxies an explicit dashboard API allowlist to one configured HTTP(S) server origin. It never exposes worker routes or accepts client-selected upstreams. Reject upstream redirects and strip forwarding/origin headers and unrelated cookies. Preserve Range/conditional headers and stream responses with request cancellation, without media buffering or local files.
- Keep sessions and static-token validation on the backend. The frontend forwards login/session cookies, checks session status before private HTML, and fails closed when the API is unavailable. Static assets and login remain public. Validate same-origin mutation requests on the frontend before stripping browser-origin headers for the backend hop; retain backend authentication/CSRF checks. Secure cookies follow frontend TLS or an explicitly configured public URL; forwarded client scheme/host headers are not trusted.
- Use dedicated Viper/pflag frontend config without importing the existing cmd package, whose dependency graph includes the backend. Defaults: listen :8081, API http://localhost:8080, public URL unset (derive from direct HTTP/TLS request). Explicit public URL supports TLS termination on another reverse proxy.
- Add frontend to build:all, docker:all and publish:app; its minimal nonroot image contains only CA certificates and the frontend binary, with no encoding toolchain.

## Risks / Trade-offs
All media crosses the frontend host network but is never stored there; preserve streaming cancellation and ranges with network tests. API unavailability must never expose private HTML or cached data. Existing embedded deployment URLs require migration to the frontend origin; retain the /dashboard/ path and provide two-host examples. The new service is a Go HTTP runtime rather than a Node runtime because the existing frontend is dependency-free static JavaScript.

## Migration Plan
Upgrade the API, deploy frontend with the API origin and its external public URL, then move the dashboard hostname/reverse proxy to the frontend. Keep media/database mounts only on the API/worker hosts. Existing backend sessions are valid only while the API process retains them; sign in again after restart. Rollback requires matching older API/frontend deployment behavior.
