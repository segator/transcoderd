# Proposal

## Why

Installed Windows workers currently start with binary self-update disabled, so publishing a release leaves their worker, service, dashboard and media tools on the previous version until somebody runs Setup. Managed installations need complete, unattended upgrades that finish accepted work and preserve the user's configuration and Windows identity.

## What Changes

- Make complete-installation updates mandatory, managed by the Windows service even when nobody is logged in. Check stable releases hourly, with bounded jitter, without confirmation, deferral or opt-out controls.
- Persist a pending update and stop new job requests on both profiles immediately after authenticating a complete eligible release. Download and verify the installer while accepted work drains; installation starts as soon as both prerequisites are met.
- Finish every accepted download, OCR, encode, verification and upload, including reserved requests, before a separate privileged helper runs Setup silently.
- Preserve the installing user's SID, server token, profile limits, schedule, pause state and working directory across a SYSTEM-initiated upgrade. Coordinate tray replacement in the user's session.
- Publish signed Windows update metadata after native release validation. Verify the installer identity, size and digest before execution.
- Persist update transactions and restore the previous installation after a failed upgrade or interrupted transaction. Keep admission blocked while an eligible update is pending and retry automatically with bounded backoff, including after restarts. Report progress, results and recovery through the English dashboard and tray.
- Keep per-profile executable self-update disabled under the desktop supervisor so one service owns the full update transaction.
- Show an unmistakable persistent update alert across the dashboard and lock every configuration control until successful completion; enforce the lock in the service and retain it across UI reconnects.

## Capabilities

### New Capabilities

None. The change extends existing update, distribution and desktop capabilities.

### Modified Capabilities

- `self-update`: Mandatory complete Windows installation updates, immediate admission blocking, authenticated staging and automatic recovery/retry.
- `windows-desktop-worker`: Unattended installation and original-owner preservation, informational update notifications and status.
- `distribution`: Signed installer metadata and native upgrade/recovery validation before release publication.

## Impact

Changes affect `cmd/windows-service`, a new Windows update helper, `worker/desktop`, the shared dashboard, the tray WebView bridge, the NSIS installer, release packaging and Windows CI. A protected update journal and last-known-good installation backup are stored beneath `%ProgramData%\\TranscoderD`; downloaded workspaces remain outside the installation snapshot.

Release signing needs an Ed25519 private key in the repository's protected CI secrets and a pinned public verification key in the application. Existing installations require one manual upgrade to the first updater-enabled installer; later eligible releases update unattended. Linux/browser-only workers and standalone worker executable updates retain their existing lifecycle.
