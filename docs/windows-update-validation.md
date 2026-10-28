# Managed Windows update validation

The updater changes are covered by Go tests, shared dashboard tests and native
Windows service/Setup tests. All UI text is English. The native pipeline uses
SVT-AV1 and FDK AAC without requiring a GPU.

| Contract | Evidence |
| --- | --- |
| Authenticity and complete payload | Signed manifest fixtures reject altered signatures, unsupported architecture/version, incomplete assets, unsafe names and missing tools; staging rejects corrupt bytes and revalidates cached installers. Production signing checks the pinned key before publication. |
| Mandatory persistent barrier | `TestMandatoryUpdateBlocksBeforeDownloadAndDrains`, `TestPendingSurvivesFailureAndRestart`, `TestInstallerWaitsForReservedRequests` and worker admission-hold tests prove persistence before download, retained limits and no installation while requests or accepted work remain. |
| Both worker profiles | Native service tests hold CPU/GPU admission, sum accepted/reserved work and reject an unreachable profile. The real service E2E keeps a second job queued while its accepted SVT-AV1 job finishes including upload. |
| Configuration lock | Shared UI tests reject submission/direct mutation requests, retain cached alerts across reload/disconnection and unlock only on authoritative completion. Native IPC and WebView tests assert the same lock. |
| Protected state and helper | Native tests check privileged file ownership, machine-wide mutex exclusion, bounded preparation retry and actual SYSTEM task registration, including a helper path containing spaces and Unicode. |
| Installation and recovery | The native service fixture injects a partial Setup failure, corrupt backup, interrupted journal, unhealthy new service and restart-required outcome; it verifies rollback, retained owner/settings/token, continued blocking and unattended retry. |
| Workspace retention | Configuration and bootstrap reject installation/staging overlaps, including existing junction aliases. Copy/restore tests verify original files and remove files introduced by failed installation. |
| Release gates | Only the main-branch release path publishes signed Windows metadata, after native service/update validation; manual debug runs never publish releases. |

Run `node scripts/windows/test-web-ui.cjs`, the relevant Go tests and
`openspec validate --all --strict` locally. `mage lint:check lint:windows` checks
both platforms through Dagger. CI runs focused Windows administration/update
unit tests before packaging; Dagger builds the shipped binaries, media bundle,
installer and fixture installers. The full native workflow retains encode,
service and update-state logs as artifacts.

For test-script iteration, maintainers can dispatch the main workflow with
`windows_debug_artifact_run` pointing to a trusted earlier Windows artifact run.
Its separate `windows-debug` check uses the current test scripts and existing
binaries. It does not replace the ordinary PR `windows-native` check or validate
new application code; code changes require a fresh normal build.

## Native execution evidence

The [isolated Windows debug run](https://github.com/segator/transcoderd/actions/runs/40651335938)
passed the ordinary installed-service SVT-AV1 E2E and every mandatory-update
scenario, using trusted fixture artifacts built from the updater implementation
and the current harness. The [ordinary PR pipeline](https://github.com/segator/transcoderd/actions/runs/40651021872)
also passed at implementation commit `22267bdc678f8be78ddbd3a5b93643c3085133c4`,
independently rebuilding and validating its own artifacts. Its Linux CI, Windows
unit, Windows build and Windows native checks all succeeded; lint and CodeQL
checks also passed. The debug result does not replace the ordinary required check.

The native regression checks also exposed and fixed Windows Scheduler XML
encoding/principal compatibility, an invalid test firewall scope, omitted journal
fields in the recovery fixture and cleanup exit-code propagation. These cases
now fail early or run in the normal upgrade/recovery suite.

## Layout and packaging checks

Windows Edge layout checks at 390px and 820px confirm the mandatory banner is
visible and the configuration fieldset is disabled, including long diagnostic
URLs without horizontal overflow. Local screenshots in
`dist/validation/windows-update-ui/` use simulated data and contain no production
credentials.

An earlier measured cached Windows build spent about 15 minutes packaging the normal
installer and 16 minutes packaging two fixture installers in parallel. FFmpeg's
AOM, SVT, x265, FDK and NVENC stages reused Dagger cache. A separate LZMA experiment
on the same bundled payload produced 683,454,001 bytes in 25m32s, versus about
869 MB with zlib; zlib remains the project default because the experiment did not
improve build time. The benchmark installer was not installed or published.

## Rollout boundary

Hosted CI invokes the real SYSTEM recovery task and simulates interrupted and
restart-required journals. It does not reboot the hosted VM or prove the boot
trigger with no logged-in user. Perform the isolated-VM drill in
[Windows desktop operations](windows-desktop.md#isolated-vm-reboot-drill) before
broad unattended rollout. Windows is never rebooted automatically by this updater.


## Pause and bounded-prefetch regression

The follow-up pause fix has Linux Go/race coverage and native Windows queue tests
for one/four encode slots, held prefetched work, Resume, paused/restored startup,
service-owned limits replacing stale server values, and maintenance draining from
pause. CI's early `windows-unit` job now includes `worker/worker` so these regressions
run before packaging. Shared UI checks and the Dagger-built tray also pass locally.
These queue tests simulate encode completion; the complete service/media E2E still
runs in the ordinary native workflow.
