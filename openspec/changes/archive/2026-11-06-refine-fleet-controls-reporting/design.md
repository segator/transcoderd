# Design

## Context

See proposal.md. Existing controls are durable per-worker commands acknowledged after a heartbeat; the local worker already supports owned-job pause. Existing summary sizes are global, and range queries only apply a lower timestamp boundary.

## Goals / Non-Goals

Keep preset API compatibility and independent frontend hosting. Historical filters select outcomes, not a reconstructed past fleet state. Resource plots remain bounded current-tab telemetry.

## Decisions

- Add optional typed range arguments to the existing dashboard path, preserving preset callers. Return range boundaries and a separate range summary, retaining existing global fields. Use UTC comparisons and explicit upper bounds everywhere. Use daily buckets, hourly for <=24 hours; bound custom ranges to 3660 days to bound response size.
- Use local calendar date inputs; convert their inclusive end date to next-day local midnight before serializing RFC3339. This handles DST through calendar arithmetic. Hide the period selector on Jobs/Library, which remain global.
- Extend the durable command with jobId and capability version 2. Keep v1 worker-wide delivery. Add a nullable UUID column and extend action constraints; server and worker validate independently to handle race conditions. Resume clears only an individual hold.
- Extend runtime task snapshots with names, controllability and per-task progress from the existing local status; old workers fall back to known dashboard jobs. This avoids tying worker cards to the overview job cap or adding per-worker database queries. Collapse explanations into tooltips or help details and separate selected-period panels from current/all-time sections.
- Use responsive two-column analytics/resource panels where useful; keep complex worker cards wide enough for controls instead of squeezing text into unreadable columns.

## Risks / Trade-offs

- [Command races] → Revalidate owned running state at application, report failures and retain acknowledgement retry semantics.
- [Older worker] → Disable individual actions until version 2; preserve worker-wide v1 controls.
- [Range confusion] → Label exact boundaries, keep period savings together, label live/all-time sections separately.

## Migration Plan

Apply additive migration 011 before serving new controls. Upgrade workers to enable individual buttons. Existing controls and old preset queries remain supported. A newer frontend rejects custom-range results whose returned boundaries are missing or differ, so an older API cannot silently relabel its preset results during a rolling deployment. Rollback binaries can ignore the added column; do not remove production command history.
