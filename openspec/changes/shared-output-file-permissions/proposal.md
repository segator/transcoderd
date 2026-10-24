# Proposal

## Why

The server currently creates upload attempts with mode `0600` and publishes the same inode as the completed media file. On shared media mounts, SMB users other than the server process cannot read these files even though the source library is accessible.

## What Changes

- Set completed output files to mode `0664` before recording job completion, including outputs finalized from legacy staging files.
- Keep other users read-only; do not grant world write access.
- Add a regression test and document the shared-output permission contract.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-transfer`: define permissions for published output files.

## Impact

This affects server-side upload finalization in `server/scheduler`, its tests, and the `media-transfer` OpenSpec capability. No API or configuration change is required.
