# Design

## Context
See proposal.md. The API owns filesystem and job state; the frontend has API access and optional OIDC. Windows service profiles already emit managedProfile and encoder telemetry. All dashboard jobs currently share one 100-row slice.

## Goals / Non-Goals
Goals: truthful playback roles, bounded queries and lists, profile-specific controls, and verified display identity.
Non-Goals: worker streaming, new codecs, persistent resource history, deleting historical workers, or replacing the existing compatible preview API.

## Decisions
- Add an authenticated jobs endpoint backed by parameterized state/search/count/pagination queries and current-assignment progress. Keep overview statistics independent. A larger global limit cannot solve starvation or pagination.
- Filter recent workers in SQL and Finalize; sort online/active first. Paginate the already bounded-by-recency worker set in the UI. Render minimal offline cards and guard save handlers.
- Return only queued sources or completed outputs from JobAssets and reject other job states. Keep rooted direct library playback available. Remove frontend preview-mode/seek/toggle code and use native controls.
- Derive fixed CPU/GPU presentation from managed profile telemetry. Preserve hidden zero inputs to submit both limit values atomically. Omit irrelevant hardware summaries and details while retaining shared RAM and explicit host/device measurement scope.
- Store bounded display claims in the authenticated encrypted OIDC payload. Serve identity locally only after backend session validation. Preserve the existing 204 API session contract.

## Risks / Trade-offs
- Original container/codecs may not be browser-decodable → show a precise original-format error; do not silently convert.
- Existing encrypted sessions have no retained identity → display a sign-in-again hint until next login.
- Offset pages may shift as jobs complete → refresh current filters and clamp empty pages to a valid page.
- Hardware is host/device-wide → identify shared Windows profiles and avoid per-job utilization claims.

## Migration Plan
Deploy API and frontend together; no database migration. Existing jobs, workers and token sessions remain compatible. Rollback both binaries if necessary.
