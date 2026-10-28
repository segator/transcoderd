# Proposal

## Why

The desktop can start prefetched encodes after Pause, because zero only disables job requests. Each profile also buffers two extra jobs. A stale server limit can replace desktop settings during startup, allowing more encodes than selected before a later reduction takes effect. Native read-only status confirmed six GPU encodes against a target of four.

## What Changes

- Pause prevents new requests and new FFmpeg starts; running encodes, verification and uploads finish. Queued work remains available for Resume, including after a Windows service restart.
- Limit admission to the effective encode target plus one pipeline job per profile.
- Make the Windows service configuration authoritative before worker admission or restored encode execution.
- Keep installation/configuration draining separate: it must finish all accepted work even when previously paused, while retaining the saved pause preference.
- Explain these states consistently in the tray and shared browser dashboard.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: bounded prefetch, encode pause and startup limit authority.
- `windows-desktop-worker`: mixed-profile pause and authoritative desktop settings.
- `self-update`: drain accepted work despite a prior user pause.

## Impact

Worker execution and local controls, Windows service startup, shared dashboard/tray wording, and regression tests. No encoder settings, media output, database schema or assignment protocol changes.
