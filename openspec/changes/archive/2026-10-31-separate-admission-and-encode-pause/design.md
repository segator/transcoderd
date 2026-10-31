# Design

## Context

The current maximum-job control can suspend active encodes through pauseMode. Individual holds already compose with the process controller's general hold. The service saves one pause preference and propagates desktop limits before startup.

## Goals / Non-Goals

Separate queue admission from explicit live encoding suspension without changing compression or adding durable checkpoints.

## Decisions

- Add an explicit worker RAM flag and startup worker.ramPaused (default false). Effective process hold combines it with legacy CLI RAM policy; encode/admission gates also respect it. Explicit RAM actions convert the runtime legacy policy to finish so Resume cannot leave an invisible legacy hold.
- Add explicit admission-limit control which persists limits and uses finish policy while respecting explicit RAM and individual holds. Keep ordinary standalone set-max-jobs legacy behavior compatible.
- Persist desktop ramPaused independently of the existing paused queue flag. Generate desktop workers with finish zero-limit policy and their saved explicit RAM flag; normalize legacy desktop strategy values to ram. No database migration.
- Add local and authenticated browser pause-encodes/resume-encodes commands. Desktop global actions target both profiles, persist successful preferences, and reapply saved intent when a profile restarts. Maintenance clears live RAM holds without changing saved preferences and blocks further mutation.
- Retain existing toggle-pause as the queue control and add toggle-encodes. Labels, help, connection state and notices describe each independently. Update the tray's allowlist and mandatory-update locks for both controls.

## Risks / Trade-offs

- Releasing one hold could accidentally release another → test all four queue/RAM combinations, independent resume and individual holds with real child processes.
- Startup settings may reopen queued work → startup flag and settings-resolution tests verify RAM gating before restored jobs start.
- RAM state is not durable → preserve the preference only; keep the existing memory/reboot explanation and assignment timeout boundary.
- Multi-profile actions may fail midway → report errors and expose observed RAM state so recovery actions remain available; never claim success on a failed profile.

## Migration Plan

Existing desktop queue pause retains finish-active meaning. Empty/finish desktop strategy migrates to RAM for the separate immediate action. Legacy standalone pauseMode and set-max-jobs remain supported. Deliver a fresh local preview installer after validation.
