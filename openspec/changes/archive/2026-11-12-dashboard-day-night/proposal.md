# Proposal

## Why

Operators use both dashboards in different lighting conditions. Frequent SSO logins, duplicate Windows worker cards and full-width concurrency controls add friction to routine monitoring.

## What Changes

- Add persistent Day and Night theme selection to the fleet and local worker dashboards.
- Keep verified fleet SSO sessions usable for up to 15 days, including across API and frontend restarts.
- Show CPU and GPU profiles of one Windows service under one worker and keep their controls independent.
- Label the concurrency controls **Concurrent encodes** and place each numeric control and its Save action together on mobile.

## Capabilities

### Modified Capabilities

- `operations-dashboard` covers fleet appearance, OIDC sessions, worker presentation and concurrency controls.
- `configuration-and-runtime` covers the shared local worker dashboard appearance.

## Impact

Fleet and shared worker web assets, frontend and API session handling, and the API database schema.
