# Proposal

## Why

Windows users need a persistent native worker that can be configured and monitored without manually launching PowerShell or editing credentials.

## What Changes

- Provide an interactive tray application with a web interface showing worker state, job progress, the local prefetch queue and the server queue, with editable server URL, credential, worker identity, encoder, job limit and schedule.
- Provide an automatically started Windows service supervising the existing bundled worker and a local per-user control channel.
- Add a guided Windows installer and uninstall/upgrade behavior, preserving configuration and accepted work.
- Build, test and publish the installer alongside the existing portable distribution.

## Capabilities

### New Capabilities

- `windows-desktop-worker`: Windows service, tray application, local administration, installation and upgrades.

### Modified Capabilities

- `events-and-progress`: expose current worker progress to the local desktop control interface.
- `job-dispatch`: provide a bounded, authenticated view of the server's queued jobs without assigning them.
- `distribution`: distribute and validate the Windows installer alongside the portable bundle.

## Impact

Worker local control and progress tracking, Windows service and desktop components, Windows package build, CI and release, and deployment documentation. Linux worker and existing portable controls remain supported.
