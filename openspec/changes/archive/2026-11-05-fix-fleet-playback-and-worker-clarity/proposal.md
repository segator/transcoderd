# Proposal

## Why
Job playback implies access to unfinished encodes while actually serving the source. Completed jobs can disappear behind the global 100-row limit, historical workers inflate fleet counts, profile controls expose irrelevant resources, and verified login identity is discarded.

## What Changes
- Restrict job playback to queued sources and completed outputs, clearly labeled.
- Use original-file playback with one native player timeline; keep the compatible preview API for existing clients.
- Query and paginate jobs by state/search before limiting results.
- Count/list workers seen within 30 days, online first, with pagination and minimal offline cards.
- Show only relevant limits and hardware metrics for fixed Windows CPU/GPU profiles; explain their shared host.
- Identify prior job failures and explicit retries separately from scan errors.
- Retain verified OIDC display claims in the encrypted session and display the signed-in identity.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `operations-dashboard`: Accurate filtered jobs, recent worker membership, relevant controls, explicit playback roles and signed-in identity.
- `media-library`: Original-only dashboard player, job playback state/role policy and clear retry provenance.

## Impact
Frontend embedded JS/HTML/CSS, frontend encrypted OIDC session and identity endpoint, dashboard repository/scheduler/API, library job assets, tests and operational references. No worker transport or database migration.
