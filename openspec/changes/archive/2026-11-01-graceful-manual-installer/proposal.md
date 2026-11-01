# Proposal

## Why
Manual Setup currently refuses upgrades with accepted work and can leave GPU admission open because preparation stops at the first busy CPU profile. Using the installed helper also prevents a new installer from improving this behavior for old installations.

## What Changes
- Use the new helper bundled with Setup to wait visibly for both profiles to finish accepted work.
- Hold admission for both profiles, resume accepted queued/suspended work, then stop the service only after verified idle.
- Fail safely on missing control or competing updates; preserve settings and work.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `windows-desktop-worker`: graceful manual upgrade wait.

## Impact
Installer, service helper/preparation, native tests and desktop operations documentation.
