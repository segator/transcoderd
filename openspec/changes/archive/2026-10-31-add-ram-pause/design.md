# Design

## Context

The Windows service owns two managed workers. Pause persists zero limits; the encode pool holds queued jobs, while active FFmpeg commands currently run to completion. Maintenance draining deliberately overrides this queue hold. Command execution already owns Windows job objects for cancellation.

## Goals / Non-Goals

Preserve the same active encoder and its compression state while suspending its compute. Only the encoding command is suspended; network coordination, downloads, verification and upload remain responsive. Durable checkpoints, chunk mode, schedule-driven suspension and cross-worker transfer are excluded.

## Decisions

- Preserve `finish` as the legacy/default mode. Add `ram` in settings and worker configuration, and a local set-pause-mode control; hot settings changes apply before limits without process restart.
- Use a command-owned process registry protected by a mutex. Serialize pause against command registration and removal. A paused gate blocks command starts; command exit removes its entry before reaping to avoid signaling recycled PIDs. Own native Windows process handles for suspension; Linux uses the owned process group with SIGSTOP/SIGCONT and observes the stop using non-reaping waitid before acknowledging pause. Cancel remains able to kill stopped processes.
- Add an optional controller only to FFmpeg encoding, leaving other command users unaffected. Count actual suspended processes in runtime status; render the relevant running encoding jobs as paused. Keep existing maximum/count fields for compatibility.
- Maintenance drain and mandatory-update admission hold resume the command registry before releasing queued encodes. Do not change the desktop's persisted pause preference.
- Apply OS transitions before reporting runtime success. Roll back already changed processes if another process fails and report errors. Retain retryable service settings on failure.
- Keep the current server assignment-timeout policy (default 24h); RAM pause is not a durable lease or reboot recovery mechanism. Document that pauses must remain within that policy rather than silently changing assignment semantics.

## Risks / Trade-offs

- RAM and VRAM stay reserved → explain in tooltip and visible pause notice.
- Native suspension is platform-specific → real-process lifecycle tests on Linux and Windows, cross-build all affected Windows binaries, and retain process-group/job cancellation.
- Startup/pause/exit races → serialize ownership and starts, test repeated transitions, cancellation and no-start while paused.
- Multiple profiles may apply controls at different times → report failed profile controls, retain pending application and retry; never claim all encodes suspended when only some are.
- Server requeues old assignments after jobTimeout → document the existing bound. This change does not add durable long-term pause leases.

## Migration Plan

Missing mode values normalize to finish, preserving installed behavior. Saving RAM pause opts in. Switching back to finish resumes active encodes but leaves queued encodes held until Resume. No database migration or encoder/profile change is required.
