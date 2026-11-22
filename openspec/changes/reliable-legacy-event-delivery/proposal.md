# Proposal

## Why

Legacy event publishing can wait indefinitely on a global worker lock and the server reports both event conflicts and internal failures as HTTP 500. A lost response can also cause an accepted event to be retried as a conflicting duplicate.

## What Changes

- Make exact event replays idempotent and report authoritative conflicts distinctly.
- Retry transient failures indefinitely with bounded individual requests and preserve job state across outages and restarts.
- Discard only a job whose assignment was explicitly rejected as expired, retaining a diagnostic log; other definitive conflicts must not be silently discarded.
- Preserve compatibility with older peers without treating ambiguous responses as definitive rejection.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `events-and-progress`: Define reliable event delivery, replay, rejection and recovery.

## Impact

Server event handling and repository transaction semantics, worker event delivery and persistence, HTTP protocol, operational diagnostics and tests.
