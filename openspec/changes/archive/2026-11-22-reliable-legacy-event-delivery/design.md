# Design

## Context

The legacy event endpoint returns HTTP 500 for both conflicts and transient errors, and only terminal completion replays are accepted. The worker retries server errors with a shared event lock and no per-attempt timeout.

## Goals / Non-Goals

**Goals:** Keep accepted work during outages; identify authoritative per-job rejections; recover uncertain deliveries safely; isolate affected jobs.

**Non-Goals:** Change the protected protocol or infer job rejection from generic 5xx errors.

## Decisions

Persist each exact notification before sending and replay it unchanged until acknowledged. The server transaction accepts an identical event replay and returns an explicit conflict for a genuinely incompatible event. Store a digest of the immutable notification payload to detect exact replay; older rows without a digest use their retained durable fields. Bound individual HTTP attempts but retry network errors, timeouts and 5xx indefinitely with backoff. Serialize per job, not across all jobs. Treat only explicit protocol conflicts as definitive; older servers' ambiguous 5xx responses remain retriable. On an explicit expired-assignment rejection, stop the affected job, log the reason, remove its workspace and release its slot; do not emit further events for it. Preserve work on ambiguous failures and other conflicts. Ensure shutdown and startup preserve and reconcile pending events.

## Risks / Trade-offs

Rolling upgrades require server-first deployment for explicit conflict responses; old servers may leave conflicts retried until upgraded. Durable replay must cover completion side effects and acknowledge only after transaction commit. A timed-out request may already have succeeded, requiring safe replay.


The worker includes the assigned-event sequence in notifications and transfer requests. The server checks it under a per-job database lock and checks again when committing an upload. Assignment-specific staging paths prevent a prior result from contaminating a later run. A stale worker is rejected before finalization or publication, and its workspace is discarded only after authoritative expiry. A conflicting notification releases its execution slot but retains the workspace and rejection reason.
