# Proposal

## Why

Operators use the worker's runtime max-jobs command to raise or lower parallel AV1 encodes without restarting the container. Today it changes only job admission, so raising the limit can leave jobs waiting at FFmpeg while the encoder pool stays at its startup size.

## What Changes

- Make positive runtime max-jobs values update both job admission and the target number of concurrent FFmpeg jobs.
- Preserve active encodes when the target is lowered; defer starting additional encodes until the running count falls below the new target.
- Keep non-FFmpeg pipeline phases at one consumer and expose active/target encoder counts in local status.
- Make max-jobs zero stop new requests while accepted work continues at the previous encoder target.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: change runtime worker control and encoding concurrency behavior.

## Impact

The worker job executor, its local control status output, configuration reference, and job-dispatch specification are affected. No server protocol or remote control endpoint is added.
