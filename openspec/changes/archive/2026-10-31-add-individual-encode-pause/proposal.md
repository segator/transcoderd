# Proposal

## Why

The current RAM-pause control applies to every encode in a worker. Users also need to suspend one CPU or GPU encode while other work continues.

## What Changes

- Add per-encode Pause/Resume controls beside the existing general control.
- Keep individual pause reasons independent of general pause, preserving manually paused encodes when the general pause is resumed.
- Route job controls to their owning profile and reject stale jobs and maintenance-time mutations.
- Resume all accepted encodes during maintenance and keep memory/lifetime limits visible.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: independently controlled active encodes and composition with general pause and maintenance.
- `windows-desktop-worker`: job-row controls, correct profile routing and maintenance protection.

## Impact

Owned process controller, local and authenticated browser controls, Windows supervisor and tray bridge, embedded dashboard, regression tests and operational references. No chunking, encoder parameter changes, checkpoint persistence or scheduling changes.
