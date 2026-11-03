# Proposal

## Why

Users need to reclaim CPU/GPU compute immediately without discarding a running encode. The existing dashboard pause only holds new assignments and queued encodes.

## What Changes

- Add selectable RAM pause for active FFmpeg encoding processes on Windows and Linux, preserving the same encoder process and output.
- Persist the pause mode in desktop settings and expose an explanatory tooltip, live paused status, and CLI/YAML configuration.
- Keep existing finish-active behavior as the default, and resume suspended encodes when maintenance drains accepted work.
- Preserve ordinary process cancellation and restricted local administration.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: selectable suspension of active encoding processes with queue and admission holds.
- `windows-desktop-worker`: persisted mode selector, tooltip and mixed-profile reporting.

## Impact

Command process lifecycle, worker encode control, desktop settings and Windows service control, embedded dashboard, operational documentation and lifecycle tests. No codec/profile changes, chunked encoding, durable process checkpointing or new network administration endpoints.
