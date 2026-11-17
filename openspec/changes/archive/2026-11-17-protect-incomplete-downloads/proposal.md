# Proposal

## Why

A source that repeatedly stops during transfer can consume many full-file downloads and leave the dashboard reporting more bytes than the file contains. The failure can be caused by source storage or network disruption, and its cause is not currently distinguishable from the worker alone.

## What Changes

- Bound repeated truncated source transfers and verification failures and report a terminal diagnostic so automatic job maintenance does not restart the cycle.
- Reset transfer progress for each new download attempt and retain integrity checks.
- Verify recovery from an intermittent interruption and permanent handling of repeated truncation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-transfer`: Bound repeated incomplete transfers and expose per-attempt progress.

## Impact

Worker download retries, progress reporting and server failure classification; no wire-format change. Existing installations must update both worker and server to receive the complete protection.
