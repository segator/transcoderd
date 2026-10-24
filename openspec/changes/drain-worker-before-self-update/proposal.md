# Proposal

## Why

When an update is discovered during a worker job-request cycle, the worker currently exits immediately. That can interrupt other jobs running in parallel, while the supervisor may repeatedly restart the old child until its cached release check expires.

## What Changes

- Stop worker job admission when a newer release is detected and let all accepted jobs finish before restarting for the update.
- Force the supervisor to refresh its release check when the child requests an update restart.
- Add regression tests for draining active jobs and refreshing the cached release result.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `self-update`: A worker drains accepted jobs before restarting for a newer release; the supervisor immediately refreshes release information after that restart request.

## Impact

The worker coordinator, job admission state, update supervisor, their tests, and the `self-update` OpenSpec capability. No CLI flags, APIs, or dependencies change.
