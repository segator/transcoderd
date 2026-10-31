# Proposal

## Why
Users want an explicit, one-time option to turn off their Windows PC after accepted transcoding work finishes. Ordinary idle, pause and maintenance must never trigger shutdown.

## What Changes
- Ask on the Windows Stop after current action whether to shut down after accepted work completes.
- Track an in-memory opt-in, drain both profiles and expose pending shutdown with cancellation through Resume queue.
- Cancel on configuration, update, worker exit or service restart.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `windows-desktop-worker`: explicitly requested shutdown after accepted work.

## Impact
Dashboard, tray, service IPC and reversible worker finishing control. No persistent shutdown setting.
