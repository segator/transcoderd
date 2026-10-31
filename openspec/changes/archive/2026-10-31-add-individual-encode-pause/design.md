# Design

## Context

The existing controller tracks owned encoding processes by job ID and serializes global suspension with start and exit. The service aggregates two worker profiles, and the WebView forwards an allowlisted JSON protocol.

## Goals / Non-Goals

Add independent active-encode controls with accurate pause reasons. Queued jobs, persistent individual pause intents across process exit, chunked resume and changes to slot accounting are excluded.

## Decisions

- Keep a general hold plus a manual hold per registered process; effective suspension is their OR. General Resume clears only the general hold. Individual controls always use RAM pause, even when the general mode is finish.
- Apply transitions transactionally with rollback of each process's prior actual state. Remove per-process intent at exit. Unknown job IDs never become future pause intents.
- Add job pause/resume to local controls and authenticated browser routes. Validate UUIDs in the worker and resolve profile ownership from authoritative snapshots in the desktop service; clients never select an OS PID or trusted profile.
- Report canPause and individuallyPaused for each job alongside existing paused state. Disable job actions while the general RAM hold is active, pending reconfiguration or configuration lock; preserve manual holds until their own Resume.
- Maintenance explicitly clears live individual holds and releases all suspended commands while preserving the desktop's saved general preference. Hold admission throughout the drain and reject new job pause commands.
- Reuse existing event delegation for dynamic rows and the restricted C# WebView allowlist, extending all update locks to the new commands.

## Risks / Trade-offs

- A job can exit between snapshot and control → report stale-job errors and never target another process.
- General and manual holds can confuse users → show the hold reason and explain general Resume in the global tooltip.
- A paused encode retains its slot and memory → show this in action help; do not launch a replacement encode into its occupied slot.
- New actions could deadlock maintenance → resume all process holds during drain and reject per-job mutations until restart.

## Migration Plan

Existing settings and general controls remain compatible. New controls are added for active encodes; individual state is live process state and disappears when the process exits. Supply an updated local preview installer after validation.
