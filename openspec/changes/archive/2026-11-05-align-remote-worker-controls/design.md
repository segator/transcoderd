# Design

## Context
See proposal.md. Workers poll the API; the frontend cannot connect to their local listeners. Local controls already separate RAM pause from queue stop. Positive CPU/GPU limits are currently lost when zero overwrites desired settings.

## Goals / Non-Goals
Goals: reuse worker-owned control semantics, durable restore limits, command acknowledgement and truthful plots/filtering.
Non-Goals: remote PC shutdown, exposing local listeners, or persistent telemetry history.

## Decisions
- Add a per-worker durable command slot with UUID, action, restore snapshot, pending/applied/failed outcome and bounded diagnostics. Worker settings reads include a pending command only for capable clients. A separate worker-authenticated acknowledgement compares the exact UUID, preventing stale completion of newer commands.
- Preserve positive settings in additive worker_settings resume columns on every positive local/fleet write. Queue resume uses the saved pair/legacy limit, including after refresh and worker restart. Queue commands persist capacity before changing runtime. RAM commands keep capacity unchanged. The worker remembers applied UUID/result while acknowledgement retries; maintenance takes precedence.
- Keep both classes and independent holds intact. New controls require online heartbeat telemetry advertising version 1. A worker pause requires at most the existing roughly five-second poll delay, then OS process suspension.
- Filter the Workers view by lastSeen against generatedAt and selected hours; keep overview fleet membership at 30 days. Paginate the filtered set.
- Reuse bounded in-tab History and SVG chart rendering in cards; relevant profile series only, explicit stale/empty display.

## Risks / Trade-offs
- Acknowledgement may be delayed → show pending until confirmed, retry idempotently.
- Server and worker upgrades are separate → disable remote actions on unsupported telemetry.
- Original last-positive settings may be absent on already-stopped legacy workers → require an explicit positive limit save, without inventing resume capacity.
- Charts begin on tab load → label the time window and never fabricate historical data.

## Migration Plan
Apply additive migration 010 with server rollout, then upgrade workers and frontend. Existing capacity endpoints remain compatible. Rollback binaries leaves additive columns/table unused.
