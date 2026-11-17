# Design

## Context

Disk 5 on jvdivx returned source read EIOs. The HTTP status is already 200 by the time the server encounters these errors. Existing workers retry a truncated response, but stop on 404. Current stream cleanup publishes a checksum even when the stream fails.

## Goals / Non-Goals

**Goals:** Prevent repeated source reads and network traffic on the affected assignment with existing workers. Allow a new assignment to retry. Do not publish a checksum from a partial stream.

**Non-Goals:** Diagnose or repair storage, protect a read that never returns, or change the deployed service during development.

## Decisions

After a non-EOF file-read error, record the job UUID and started-event ID in a mutex-protected, time-bounded in-memory guard. Reject requests for that same assignment with the established 404 response so existing workers stop. The guard expires after 24 hours and is lost on server restart; the next attempt can read once and reestablish it. Never block for a client write error or cancellation. A new started-event ID permits retry without affecting another assignment. Only publish checksums after the advertised number of bytes has been read and written. Publish synchronously into a mutex-protected map, replacing the asynchronous channel to eliminate its data race and publication lag.

## Risks / Trade-offs

- HTTP 404 means legacy workers report source not found; server logs retain the actual I/O error.
- Each affected job can attempt one read; no disk-wide circuit breaker is inferred from file paths.
- Process restart loses the guard and may cause one further read per assignment.
