# Tasks

## 1. Playback and scanner provenance
- [x] 1.1 Restrict job assets by latest Job state and role; verify library and HTTP regressions including phase completion, failure and missing output.
- [x] 1.2 Replace dashboard playback with original files and native controls, label queued/completed actions and prior encode retries; verify browser behavior and update media reference.

## 2. Job querying and recent fleet
- [x] 2.1 Add filtered paginated jobs repository/scheduler/API and frontend navigation; verify more-than-100 queue starvation, search and paging in unit/SQL/API tests and document wire format.
- [x] 2.2 Filter 30-day worker membership, sort online/active first, paginate and remove offline controls; verify freshness boundaries and UI behavior.

## 3. Worker profiles and session identity
- [x] 3.1 Present fixed CPU/GPU controls and hardware metrics by profile, retain atomic limits and explain shared host; verify profile and offline UI regressions and docs.
- [x] 3.2 Retain verified bounded identity claims in encrypted OIDC sessions and display authenticated identity; verify cross-replica identity, revocation, anonymous access and token fallback documentation.

## 4. Integrated verification
- [x] 4.1 Run relevant Go, Node and browser checks plus openspec validate --all --strict; review implementation against all delta scenarios and record results.

## Workflow follow-up
- Synchronize completed delta requirements and archive this change after verification.
