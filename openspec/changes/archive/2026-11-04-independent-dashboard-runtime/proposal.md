# Proposal

## Why
The operator will deploy the frontend on another host with access only to the TranscoderD API. The current server-embedded frontend cannot satisfy that deployment boundary.

## What Changes
- **BREAKING**: Move dashboard pages, assets and PWA serving out of the server into an independently runnable frontend executable/container.
- Keep authentication, scheduling, telemetry, rooted library access, probing, preview transcoding and job intake on the server API.
- Use a frontend API proxy so browser sessions and playback remain on one origin without filesystem mounts, database credentials or worker credentials on the frontend host.
- Add a protected session-status endpoint and retain the existing dashboard API paths; preserve English UI, PWA privacy, streaming/ranges and synchronization.
- Include frontend build/publication targets, deployment configuration and a two-process local preview.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `operations-dashboard`: independent frontend delivery and API-only backend, authentication and PWA ownership.
- `configuration-and-runtime`: frontend settings, startup and graceful shutdown.
- `distribution`: standalone frontend binary/container and versioned publication.

## Impact
New frontend package and runtime; server web route removal/session endpoint; build, CI, tests and operator references. Existing worker API, media roots, migrations and encoding behavior remain compatible.
