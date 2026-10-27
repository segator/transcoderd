# Design

## Context

See `proposal.md` for motivation. The existing `cmd/windows-service` supervisor owns both CPU/GPU child processes and launches each with `--noUpdates`. Its local `drain` control already stops admission without changing the server's persisted concurrency; status exposes active and reserved jobs. The current `prepare-upgrade` operation rejects an install while work remains instead of waiting for it.

The NSIS installer already supports `/S` and upgrades the complete installation, but its upgrade flow derives the owner SID from the current process and writes per-user shortcuts and autorun entries. Executing that flow as SYSTEM would change the owner and touch SYSTEM's user settings. Some failure paths show unconditional message boxes. The tray uses a user-session `Local\\` quit event and local `wwwroot` assets; a SYSTEM process in session 0 cannot use the current quit command to coordinate every interactive tray instance.

The release workflow creates the GitHub release before Windows finishes building and validating. No authenticated complete-update descriptor, external installation helper, transaction journal or rollback mechanism exists today.

## Goals / Non-Goals

**Goals:** one service-owned lifecycle for the whole installation, mandatory updates that finish accepted work, recoverable file replacement, a durable admission barrier and clear English UI status. Automatic progression must work with the tray closed, outside the admission schedule and before user logon. A worker with an authenticated pending update must not accept new work until successfully updated.

**Non-Goals:** changing standalone Go self-update, updating Linux images, accepting third-party installer URLs, enrolling unconfigured machines, changing encoding profiles, or automatically restarting Windows. The first installation of this capability remains an administrator-run bootstrap upgrade.

## Decisions

### 1. Mandatory service state machine

Use a separate persisted pending-update and transaction record. Complete managed updates are compulsory: there are no notification-only, disabled or deferral policies. Checks run at startup and hourly with random jitter up to five minutes, independently of worker admission. Explicit check-now bypasses the interval without cancelling an existing update. Check the latest stable project release and compare semantic versions.

Present a persistent, high-contrast mandatory-update panel above every page showing target version, state, remaining jobs, progress and retry time. Disable the complete settings fieldset, save, token visibility and pause/resume actions, with a visible locked explanation. Guard handlers as well as DOM controls; enforce the same mutation rejection in the service before validation/persistence. Keep last-known pending state across reconnects and reloads and leave settings disabled until authoritative status is known. Polling and read-only job monitoring remain usable.

Expose `checking`, `pending`, `downloading`, `draining`, `installing`, `verifying`, `recovering`, `retrying`, `updated`, `failed` and `restart-required` states with installed/target version, transfer progress, remaining accepted jobs and next retry time. Requests use bounded timeouts. Failed checks, staging and installation retry automatically with 15-minute, one-hour and then six-hour delays; six hours is the cap. Do not permanently suppress a failed version or require manual retry. Check for a newer authenticated target on each retry and supersede the previous target when possible.

Authenticating complete eligible release metadata establishes a pending update. Persist that barrier before any lifecycle transition, immediately stop both profiles from initiating new requests, and start downloading the installer while accepted work drains. There is no five-minute grace period, mandatory button, confirmation or postponement. Installation begins only when staging verification and complete drain both succeed. Network/storage failure after establishing the pending update keeps admission blocked during automatic retries; a failed check before any eligible target is known does not create a block.

A required confirmation dialog and optional update policies were rejected because the fleet must update without user participation. Downloading before blocking admission was rejected because the user requires no new work as soon as an authenticated update is pending. The existing binary updater was rejected for managed installs because it cannot replace the tray, service and tools together and could race across profiles.

### 2. Signed metadata published only after validation

Publish `windows-update.json` plus a detached Ed25519 signature after native validation. Metadata binds schema version, release/tag, Windows architecture, exact installer asset name, installer byte size and SHA-256, and expected installation file hashes. Resolve installer URLs through the project's matching GitHub release assets, not arbitrary client input. Download only over HTTPS with bounded redirects to GitHub's release delivery hosts; cap metadata and installer sizes and reject invalid paths or duplicate manifest entries.

Generate a release-signing key during implementation bootstrap, store the private key directly in a protected GitHub Actions secret `WINDOWS_UPDATE_SIGNING_KEY`, and commit only the public verification key. Never print the private material or export it in artifacts. Fixture tests use their own keys. Missing signing configuration fails publication of automatic-update eligibility; clients continue normal work if signed Windows assets are not yet available. Authenticity is required because the service executes the downloaded installer as SYSTEM. A checksum obtained alongside an arbitrary download is insufficient for that trust boundary; a paid Authenticode certificate is not a prerequisite for this design.

After authenticating complete metadata and establishing the admission barrier, stage installer, verified manifest and the helper in a fresh protected directory under `%ProgramData%\\TranscoderD\\updates` while draining proceeds. Check free space for installer, installation and rollback snapshot. Reject reparse points and ensure ordinary users cannot replace the stage or transaction files. Staging failures retain the pending barrier and retry automatically. The helper independently verifies the signed descriptor and staged installer immediately before execution.

### 3. Drain the complete CPU/GPU pipeline

On authenticating a complete eligible release, acquire the lifecycle/maintenance guard and drain both profiles through local control. Coordinate this guard with existing pending reconfiguration and manual installer preparation. Already-saved pending settings may finish applying under the admission hold; reject further encoding configuration and limit changes while an update is pending. Poll both profiles until `activeJobs == 0` and `reservedJobs == 0`. Retain progress visibility while downloads, OCR, prefetched encodes, verification and uploads finish. Missing control responses are errors, not idle evidence. Do not kill work or impose an encode timeout.

Keep CPU/GPU job limits, `Paused`, schedule, connection and working directory intact. Restore admission from those settings only after a successful verified update. On service/child restarts, read pending state first and start profiles with an internal admission hold so loading server limits cannot reopen requests before the control pipe is ready. Accepted persisted jobs can resume to completion under that hold. Rollback preserves the hold and continues automatic retries. Continue launching child workers with `--noUpdates`; complete managed updates are exclusively owned by the service.

### 4. External helper and durable recovery

Build a small `cmd/windows-updater` helper and copy it outside the installation into the protected transaction stage before launch. Pass a fixed, protected transaction identifier; do not expose executable paths or free-form commands through the service API. Use a machine-wide update mutex. Persist an atomic journal and a complete previous-installation snapshot, including service registration/owner metadata, before the helper stops the service and runs Setup with `/S`.

Register a fixed SYSTEM boot-recovery task before binary replacement, pointing to that protected helper and journal. This permits recovery even if a crash leaves the service executable unavailable. The helper is independent of service shutdown. The service checks for an unfinished transaction on startup and exposes health/status while holding job admission closed.

After Setup exits, verify installed hashes against the signed manifest, start the service and verify its version and restricted control channel. Commit the transaction, release maintenance and retain only one successful previous installation. If installation or health validation fails, restore the previous files and service registration, validate the old service in maintenance and retry updating automatically while admission stays blocked. Preserve all configuration and workspaces outside the installation directory. If rollback cannot be proven healthy, remain stopped and let the protected recovery helper retry. Never initiate a Windows reboot; persist a restart-required transaction if prerequisites require one and let boot recovery validate it.

A helper with no durable boot recovery was rejected because power loss can occur while the service executable is being replaced. Direct self-replacement from the service was rejected because Setup must stop and replace that same executable.

### 5. SYSTEM upgrades retain the original user

Persist the initial installing owner SID in protected machine installation metadata and preserve it from the existing service registration for migration. Use it when registering the new service; do not replace it with SYSTEM or whichever administrator runs Setup. On an upgrade, retain the existing per-user autorun/shortcut registration rather than create entries under SYSTEM's HKCU. Preserve destination and data paths.

Make every unattended error path log and return a nonzero exit code without a message box; recognize prerequisite exit codes and suppress reboots. Reuse the same verified install path for interactive and unattended upgrades. The service's existing prepare-upgrade check remains a final guard even after the helper drained the profiles.

Use session-aware IPC to request that known installed tray instances exit. Identify processes by original owner SID, session and full installed executable path; after a bounded graceful timeout, stop only verified tray instances if necessary. After successful install or rollback, launch the tray as the original owner in an active session using the supported Windows user-token APIs. When no such session exists, the retained logon startup handles it later. All worker encoding remains in the service. Reconnect/update UI status must survive the brief service restart without displaying a misleading permanent connection error.

### 6. Native release evidence

Extend Windows CI with two versioned fixture installers and a local fixture release feed. Unit tests inject time, release responses, control state and installer outcomes. Native tests exercise the real SYSTEM service and silent Setup: detect a newer fixture release while a real SVT-AV1 job runs, prove no later assignment starts, finish the output upload, install, verify version and original user's pipe access, and compare retained settings/limits. Inject invalid signatures and an installer failure; exercise journal recovery through the actual boot-recovery entry point without rebooting the hosted runner. Include a reboot-validation procedure for an isolated VM to exercise the boot trigger itself before rollout.

Test endpoints and keys are restricted to explicit test binaries or protected fixture configuration on isolated CI. The production service never accepts an endpoint override through the dashboard. Do not install experimental builds into the user's current production worker during development.

## Risks / Trade-offs

- Large installer and backup storage cost → download while accepted work drains, enforce free-space checks, retain one rollback snapshot and clean completed stages.
- Missing signing secret/key mismatch → fail update-metadata publication and continue the installed version; prove signature verification with CI fixture keys before bootstrap rollout.
- Locked tray files and Windows session isolation → use session-aware tray shutdown/relaunch and native multi-identity tests.
- A failed update or unavailable download leaves the worker idle → retain the authenticated pending target, show the next retry and retry automatically with bounded backoff; install a newer authenticated target when available. This is an intentional consequence of mandatory updates.
- Long-running or paused accepted jobs delay an upgrade → show remaining jobs and wait; never sacrifice their output for an update deadline.
- Future configuration migrations make downgrade unsafe → require backward-readable settings for this release and include data compatibility in future updater manifest/schema changes.

## Migration Plan

1. Provision release signing, add the public key and publish updater-enabled builds only after native upgrade/recovery tests pass.
2. Existing managed installs run the new installer once. Read the original service owner and preserve settings; the managed updater is mandatory without an opt-in or opt-out setting.
3. Validate a signed follow-up release on an isolated Windows VM, including no-user-logon and interrupted-install recovery, before broad unattended rollout.
4. Later stable releases immediately block new requests once authenticated, download while accepted work drains, and install without confirmation. Failed attempts restore the previous complete installation in maintenance and retry automatically; only a verified current installation releases the admission hold.

## References

- [Microsoft: Interactive Services](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services) documents session isolation and using a separate user-session application with IPC.
- [NSIS: installer command line](https://nsis.sourceforge.io/Docs/Chapter3.html) documents silent `/S`; [silent installer behavior](https://nsis.sourceforge.io/Reference/SilentInstall) notes that scripts must still handle message-box behavior explicitly.
