# Design

## Context

The fleet dashboard and worker dashboard ship independent stylesheets. The Windows tray embeds the shared worker assets. A Windows service starts managed CPU and GPU profiles that register separately with the API. A verified OIDC login uses an encrypted frontend cookie and a server-side session that currently lasts up to 12 hours and lives in API process memory.

## Decisions

- Set the selected theme before loading styles and keep the choice in local storage for each dashboard origin. Use the existing first-visit palettes. Include the fleet theme asset in the public PWA manifest so offline pages and updates can resolve it.
- Combine managed profiles in the fleet renderer using their CPU/GPU suffixes and matching host identity. Keep per-profile names as keys for jobs, charts and commands. Retain the last confirmed identity in the current tab for up to 30 seconds during a missing sample for the same worker address. This supports existing Windows worker versions and independent control acknowledgements.
- Issue a persistent browser cookie bounded by the verified OIDC API session's 15-day expiry. The existing API login remains compatible with older servers. Store a keyed digest of each opaque OIDC session and its expiry in PostgreSQL so every API process can check and revoke it. Keep login-time verification of the provider token and the shared frontend cookie key.

## Migration Plan

The API applies an additive migration on startup. Deploy the API and frontend to enable 15-day sessions for new SSO logins. Older API instances continue issuing sessions with their existing expiry. The fleet grouping uses existing worker telemetry and works without upgrading Windows services.
