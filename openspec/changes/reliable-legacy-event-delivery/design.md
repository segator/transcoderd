# Design

## Context

The legacy event endpoint returns HTTP 500 for both conflicts and transient errors, and only terminal completion replays are accepted. The worker retries server errors with a shared event lock and no per-attempt timeout.

## Goals / Non-Goals

**Goals:** Keep accepted work during outages; identify authoritative per-job rejections; recover uncertain deliveries safely; isolate affected jobs.

**Non-Goals:** Change the protected protocol or infer job rejection from generic 5xx errors.

## Decisions

Persist each exact notification before sending and replay it unchanged until acknowledged. The server transaction accepts an identical event replay and returns an explicit conflict for a genuinely incompatible event. Compare durable event fields, ignoring transport-only metadata. Bound individual HTTP attempts but retry network errors, timeouts and 5xx indefinitely with backoff. Serialize per job, not across all jobs. Treat only explicit protocol conflicts as definitive; older servers' ambiguous 5xx responses remain retriable. On an explicit expired-assignment rejection, stop the affected job, log the reason, remove its workspace and release its slot; do not emit further events for it. Preserve work on ambiguous failures and other conflicts. Ensure shutdown and startup preserve and reconcile pending events.

## Risks / Trade-offs

Rolling upgrades require server-first deployment for explicit conflict responses; old servers may leave conflicts retried until upgraded. Durable replay must cover completion side effects and acknowledge only after transaction commit. A timed-out request may already have succeeded, requiring safe replay.


The server checks the current assignment before accepting a new notification, including completion. A stale worker is rejected before finalization or any upload side effect. Its workspace may be discarded only after an authoritative expiry response.
