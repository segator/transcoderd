# Tasks

## 1. Acknowledged remote controls
- [x] 1.1 Add durable commands and positive-limit restoration; verify memory/PostgreSQL round trips, legacy limits and stale acknowledgements.
- [x] 1.2 Add protected API/proxy delivery and worker application; verify real process pause/resume, independent holds, queue stop/resume, acknowledgement retry and capability/maintenance rejection.
- [x] 1.3 Replace ambiguous pause action with separate reversible controls and pending/error state; verify browser behavior and document API/operational semantics.

## 2. Worker visibility and resources
- [x] 2.1 Filter Workers by selected reporting period and paginate the filtered set; verify 24h/7d/30d browser boundaries and update dashboard docs.
- [x] 2.2 Add profile-aware compact resource history; verify bounded samples, missing/stale values and browser graphs, with documented sampling scope.

## 3. Delivery
- [x] 3.1 Run Go/race/PostgreSQL/Node/browser checks, strict OpenSpec validation and review against delta scenarios.

## Workflow follow-up
- Synchronize specs and archive the verified change.
- Create a PR containing all requested frontend fixes, resolve CI failures, merge only after green checks and publish the normal release with validated assets.
