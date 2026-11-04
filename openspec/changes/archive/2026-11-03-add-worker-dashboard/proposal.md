# Proposal

## Why

Operators need to tune each CPU or GPU worker from the server and understand real fleet progress and storage savings without inspecting containers or database tables.

## What Changes

- Serve an embedded, responsive operations dashboard with live job progress, queue health, compression, completion history, worker capacity, and recent failures.
- Protect dashboard data and mutations with a separate configurable static token, defaulting to `encoding123`, and an expiring browser session.
- Let operators persist individual worker concurrency limits from the dashboard and have running workers synchronize changes before requesting more work.
- Report each worker's configured encoder, admission status, and actual encode concurrency in heartbeat telemetry.

## Capabilities

### New Capabilities

- `operations-dashboard`: authenticated monitoring and individual worker capacity management.

### Modified Capabilities

- `job-dispatch`: synchronize server-desired concurrency during runtime while preserving accepted work.
- `persistence`: retain optional worker runtime telemetry alongside presence.

## Impact

Server HTTP routes, embedded browser assets, aggregate database queries and an additive migration, worker heartbeat/client/coordinator, tests, and operational references. Existing worker API credentials remain separate from dashboard credentials. Each worker retains its configured encoder; capacity controls apply independently to CPU and GPU workers by stable name.
