# Proposal

## Why
A completed previous Windows update journal can clear a newer pending update after download and drain, falsely reporting success on the installed old version. This violates the existing mandatory-update admission contract.

## What Changes
- Restrict completed-journal recovery to the same pending release and authenticated payload.
- Retain newer pending targets and continue draining/installing.
- Add regression coverage for consecutive updates and retain interrupted-commit recovery.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
None: restores the existing self-update contract.

## Impact
Managed update coordinator and tests; no configuration or protocol change.
