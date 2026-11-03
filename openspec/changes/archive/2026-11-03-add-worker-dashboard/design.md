# Design

## Context

See proposal.md for motivation. Workers currently have one configured encoder, a dynamically adjustable encode pool, a bounded prefetch buffer, and durable server-side `maxActiveJobs`. The coordinator restores that setting once at startup. PostgreSQL already retains final sizes/probes, projected status, progress, and ordered phase events.

## Goals / Non-Goals

**Goals:** expose useful real measurements, durable individual capacity controls, and safely isolated dashboard authentication without adding a frontend build or remote dependencies.

**Non-Goals:** changing encoder quality profiles, interrupting accepted work, hardware utilization measurements without telemetry, and implementing OIDC now. Each named worker keeps its configured CPU or GPU encoder.

## Decisions

- Embed plain HTML, CSS, JavaScript, and accessible SVG charts in the Go server. Keep all assets local and use a restrictive content security policy. Existing Go deployments need no Node runtime.
- Separate dashboard authentication and routes from the worker API. `web.dashboardToken` defaults to `encoding123`; a successful login creates an opaque, expiring HttpOnly SameSite browser cookie backed by a bounded in-memory session registry. Logout revokes that session; server restart revokes all sessions. Mutations require a same-origin custom request header. A dashboard bearer token can also authenticate API clients. Tokens never appear in URLs, HTML, logs, or browser storage. This route middleware is the boundary for later OIDC integration.
- Read dashboard aggregates in PostgreSQL, with bounded recent job/activity lists, rather than loading every job's history per refresh. Offer 24-hour, 7-day, and 30-day periods, UTC time buckets, all-time queue/storage totals, and explicitly labelled period metrics. Only finalized completed jobs contribute to compression; unavailable progress or duration stays unavailable. Stale telemetry is visibly marked.
- Extend Ping with optional runtime telemetry (encoder, CPU/GPU kind, admission/schedule state, actual/desired encode counts), keeping older workers compatible. Add a nullable JSONB column to workers; durable desired limits remain in the existing logged worker_settings table.
- Poll desired settings before each normal admission evaluation, including while paused, under the same settings mutex as local controls. If synchronization fails, skip new requests and retry on the next cycle. Accepted work keeps running. Preserve the existing process-local updater drain. Save first, report desired vs observed until the worker acknowledges through its next heartbeat.

## Risks / Trade-offs

- [The shared dashboard token grants operations access] → Separate it from worker credentials; use a configurable secret, revocable browser sessions and CSRF protection, and document HTTPS deployment.
- [Older workers do not poll settings] → Display telemetry as unavailable and document server-first rollout followed by worker upgrade.
- [Queue phases can complete before the overall job] → Determine terminal states only from Job-level notifications; use phase progress separately.
- [An output may grow or lack valid sizes] → Keep real signed savings and exclude missing sizes from percentage denominators.
- [Polling adds database work] → Aggregate in SQL, bound detail lists, use existing status projections and targeted indexes, and poll every five seconds only while visible.

## Migration Plan

Deploy the server first to apply the additive telemetry migration and expose the dashboard, then upgrade workers. Existing worker API authentication stays unchanged. Rollback workers before server if necessary; leave additive columns and indexes in place.
