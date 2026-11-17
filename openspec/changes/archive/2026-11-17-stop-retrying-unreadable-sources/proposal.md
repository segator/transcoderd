# Proposal

## Why

A failing source disk returned read errors after gigabytes had been streamed. Existing workers restarted the transfer repeatedly, consuming bandwidth and stressing the disk.

## What Changes

- Refuse further download requests for an assignment after the server observes a source read failure, including for existing workers.
- Publish checksums only for fully streamed downloads and preserve retries after client disconnects.
- Allow a fresh assignment to retry a source after operator intervention.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-transfer`: Stop repeatedly serving a source after a confirmed read failure and avoid publishing a partial checksum.

## Impact

Server streaming and scheduler; the response to subsequent requests uses HTTP 404 for compatibility with deployed workers. The guard is per process and per assignment; server restarts can permit one new read attempt.
