# Proposal

## Why

Windows workers update automatically but users cannot see the changes being installed or identify what changed in their installed version.

## What Changes

- Add release notes beside the desktop update check, scoped to the pending upgrade or the installed release.
- Report unavailable or incomplete notes without interfering with the mandatory update lifecycle.

## Capabilities

### New Capabilities

### Modified Capabilities

- `self-update`: Expose the relevant release notes in the Windows worker dashboard.

## Impact

Windows desktop dashboard assets, GitHub release metadata access, tests and user documentation. No update eligibility or installation changes.
