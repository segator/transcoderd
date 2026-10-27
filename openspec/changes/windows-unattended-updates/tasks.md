# Tasks

## 1. Mandatory update state, release authenticity and staging

- [x] 1.1 Add separate persisted mandatory-update/status models with authenticated pending target, admission barrier, failed-attempt tracking and next retry; verify migration, restart persistence, rejection of opt-out/deferral requests and public token redaction with Go tests, and document fields in the desktop protocol reference.
- [x] 1.2 Implement a versioned Ed25519-signed Windows manifest describing the exact installer and expected installed files; verify valid fixtures and rejection of bad signatures, digests, sizes, paths, architecture and non-newer/draft/prerelease versions with unit tests.
- [x] 1.3 Provision the protected release signing secret without printing private material, commit the public verification key and document bootstrap/rotation; verify a CI signing probe with the public key and prove missing signing configuration cannot publish eligible metadata.
- [x] 1.4 Implement hourly independent release checks, bounded jitter/timeouts, explicit check-now and automatic retries capped at six hours; verify clock-injected mandatory progress, newer-target supersession, missing Windows assets and startup without a tray.
- [ ] 1.5 Stage and revalidate installer/metadata in protected directories with progress and free-space checks while the pending admission barrier remains active; verify slow/failed downloads, restart interruption, storage failure, reparse points and unauthorized-file replacement, and document storage/cleanup requirements.

## 2. Drain and coordinate both worker profiles

- [x] 2.1 Persist the pending update and establish the admission barrier immediately after authenticating a complete eligible release, then download and drain in parallel; verify there is no grace period, confirmation, deferral or user action required.
- [x] 2.2 Coordinate update maintenance with pending configuration, manual Setup and pause/limit controls; verify concurrent lifecycle requests yield one transaction and actionable English responses.
- [ ] 2.3 Drain CPU/GPU through local runtime controls and wait for all active and reserved work, including uploads and prefetched jobs; implement the internal startup admission hold and verify no installer starts with nonzero counts/unreachable profiles, no child restart reopens requests and persisted server limits stay unchanged.
- [ ] 2.4 Restore runtime admission only after successful update verification from the user's retained limits, pause and schedule; verify failed staging/install/rollback keeps the barrier, paused/outside-schedule cases remain correct and per-profile settings persist, and document the accepted-work boundary.

## 3. Recoverable installation helper

- [ ] 3.1 Add a Windows helper built and staged outside the installation with a machine-wide mutex, atomic transaction journal and protected complete backup; verify crash-point journaling, stage permissions, expected-file validation and duplicate-transaction exclusion.
- [ ] 3.2 Register and invoke a fixed SYSTEM boot-recovery task before installation and gate service admission on unfinished transactions; verify the real recovery entry point restores or verifies interrupted fixtures without relying on the service executable being available.
- [ ] 3.3 Execute only the authenticated staged installer silently, verify installed hashes and service/pipe version, then commit or restore the previous files and service registration in maintenance; verify successful install, partial installer failure, unhealthy new service, recovery failure and restart-required outcomes without releasing the barrier prematurely.
- [ ] 3.4 Retain one previous installation, clean completed stages and persist retry state; verify cleanup cannot delete configuration/workspaces and failures automatically retry without permanent version suppression or manual action, and document recovery/diagnostic procedures.

## 4. SYSTEM-safe installer and tray lifecycle

- [ ] 4.1 Preserve the original installing SID from protected installation metadata or existing service registration; verify interactive and SYSTEM upgrades retain original-owner pipe access, installation path, user autorun and shortcuts while avoiding SYSTEM's HKCU.
- [ ] 4.2 Make silent NSIS failures return logged exit codes with no dialogs or automatic reboot, and package the update helper from Dagger; verify silent success/failure and prerequisite exit-code handling on a native Windows runner.
- [ ] 4.3 Add session-aware graceful shutdown and verified relaunch of the installed tray for the original owner; verify open/closed tray, active-user and no-user-session paths, including safe fallback for a nonresponsive tray and no interference with encoding processes.
- [ ] 4.4 Expose restricted check-now and English UI/tray status with a persistent prominent mandatory-update alert across pages, versions, progress, remaining jobs and next retry; lock every configuration input/action in the UI and service, verify keyboard/direct-request/reconnect/reload cannot bypass it, test native WebView behavior, and document the one-time bootstrap.

## 5. Release metadata and end-to-end evidence

- [ ] 5.1 Extend Dagger/Windows CI to build two reusable versioned fixture installers and a signed local test release feed with test-only trust/configuration; verify the production service cannot accept feed or executable overrides through dashboard controls.
- [ ] 5.2 Run a real SYSTEM service SVT-AV1 CPU job while an update arrives; hold the installer download open and prove no new requests begin after metadata authentication, the output uploads successfully, Setup automatically upgrades all components and original owner/settings/limits remain usable afterward.
- [ ] 5.3 Add native invalid-metadata, partial-install failure and interrupted-journal recovery scenarios; verify rollback preserves data and admission blocking, retries complete without user action and newer targets supersede failing ones, and document an isolated-VM reboot/no-logon rollout drill for the boot trigger itself.
- [ ] 5.4 Publish installer, installed-file metadata and signature only after native upgrade/recovery validation; verify release identity/digests and failure gates, and update Windows distribution and release documentation.
- [ ] 5.5 Run relevant Go tests, lint, shared UI tests, Windows cross-build and required native workflow checks; run `openspec validate --all --strict` and compare implementation against every delta scenario before marking the change complete.

## Workflow follow-up

- Keep implementation in a dedicated Windows updater branch/PR so the existing dashboard PR can be reviewed independently.
- Review the implemented behavior and test evidence, then archive this change and synchronize its approved capability deltas.
