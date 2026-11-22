# Tasks

## 1. Server protocol

- [x] 1.1 Add transactional exact replay handling and explicit conflicts for incompatible notifications, preserving completion side effects.
- [x] 1.2 Test duplicates, conflicting events, lost responses and transient server failures.

## 2. Worker delivery

- [x] 2.1 Persist pending events before transmission and recover unchanged events after restart.
- [x] 2.2 Bound each attempt, retry transient failures indefinitely, and serialize only within each job.
- [x] 2.3 Stop the affected job, log the reason, discard its workspace and release its slot on explicit assignment expiry.
- [x] 2.4 Test outages, stalled responses, retry recovery, conflicting events, concurrent jobs and restart recovery.

## 3. Verification and documentation

- [x] 3.1 Update protocol and operational documentation, including server-first rollout.
- [x] 3.2 Run relevant Go tests and strict OpenSpec validation.

## Workflow follow-up

- Archive after implementation and review.
