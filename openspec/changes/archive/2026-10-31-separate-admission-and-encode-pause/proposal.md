# Proposal

## Why

The general Pause button currently mixes stopping queued work with suspending active encodes depending on a setting. Users need two explicit, independent actions: let current encodes finish, or suspend them immediately in RAM.

## What Changes

- Keep Stop after current / Resume queue separate from Pause encodes / Resume encodes.
- Preserve both preferences independently across desktop restarts and updates; restored work remains held without implying a durable encoder checkpoint.
- Keep individual job controls and manual pause reasons independent.
- Make RAM the desktop encoding-pause strategy; retain legacy standalone CLI zero-limit policy for compatibility.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: independent admission and explicit RAM holds.
- `windows-desktop-worker`: two dashboard/tray controls and independently saved preferences.

## Impact

Worker control and startup configuration, protected desktop state, optional browser controls, tray bridge, UI, lifecycle tests and preview installer. No encoder argument or process-suspension mechanism changes.
