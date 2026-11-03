# Design

## Context

The encode pool counts jobs separately from admission. Zero currently leaves the pool enabled. Windows config contains separate CPU/GPU targets but worker startup loads server maxima unconditionally. The active keep-encoder-prefetched delta describes a two-job buffer that is implemented but not yet synchronized into the main admission spec.

## Goals / Non-Goals

**Goals:** Enforce chosen profile targets, pause queued FFmpeg starts, retain accepted jobs, reduce prefetch, preserve mandatory update draining.

**Non-Goals:** Suspend FFmpeg processes, cancel uploads, change codecs, add buffer configuration or alter schedule semantics.

## Decisions

- Add a pause gate inside the encode pool under the same mutex as slot allocation. Keep its positive target for status and Resume; zero operator limits close the gate. Initialize it closed for zero-startup configurations.
- Separate local Drain from operator zero. Drain and the managed admission hold unblock the encode pool so accepted work completes. Neither changes persisted pause/limits. Mandatory update checks remain independent.
- Use one extra accepted job per profile. The ceiling includes all pipeline work and reservations, so blocked uploads cannot grow the queue indefinitely. Restored excess work is retained and stops further admission until below the ceiling.
- Mark service-owned profiles using a private child environment flag before Run. Their startup settings synchronization writes the local desired limit instead of replacing it with server state. CLI/container workers retain server override precedence.
- Display actual active encodes without clamping, including when a target was lowered while encodes run. Use explicit pause status and distinct drain wording; fix tray Resume for mixed profiles by restoring saved profile limits.

## Risks / Trade-offs

- A one-job buffer may expose slow preparation/upload more often, but bounds disk use and matches the requested smaller queue.
- Active work above a newly lowered limit must finish safely. No further encode starts while active count is at or above target.
- Pause changes the meaning of operator zero for queued FFmpeg work. Document the separate maintenance drain and test updater/reconfiguration compatibility.
