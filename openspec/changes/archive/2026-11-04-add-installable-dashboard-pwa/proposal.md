# Proposal

## Why
Operators want to install the web dashboard as an application and reopen it conveniently from desktop/mobile. Installation and offline handling must preserve the existing authentication and private-data boundaries.

## What Changes
- Add an embedded web manifest, correctly sized branded PNG/maskable/Apple icons, installation metadata and progressive install controls.
- Register a dashboard-scoped service worker that caches only an explicit public asset allowlist and presents a public reconnection page when navigation cannot reach the server.
- Keep authenticated pages, credentials, APIs, mutations and media out of persistent caches and offline replay.
- Version public caches by embedded content and offer a user-triggered reload after updates.
- Run a local preview with clearly identified example data for UI review and document installation and HTTPS requirements.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `operations-dashboard`: installable PWA, scoped public-only caching, offline/reconnection and update behavior.

## Impact
Embedded server web assets/routes/CSP, small PWA lifecycle client, docs and HTTP/service-worker/browser tests. Existing session authentication remains unchanged. No production deployment, push notifications, background job submission or offline job processing are added.
