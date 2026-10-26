# Proposal

## Why

When all configured job slots are occupied, a worker cannot request and prepare another source while its encoders are busy. This leaves encoding capacity idle after an encode finishes, even though upload and other pipeline work could overlap.

## What Changes

- Keep the configured job maximum as the FFmpeg encode concurrency target.
- Admit a small, fixed pipeline buffer beyond that target so the worker can prepare the next job while encodes and output transfers are in progress.
- Keep the buffer bounded and preserve zero-job admission control, schedules, and serial non-FFmpeg stage consumers.
- Report the configured encode target separately from the larger admission ceiling.

## Capabilities

### New Capabilities

### Modified Capabilities
- `job-dispatch`: define bounded admission lookahead beyond the encode target while preserving a zero limit.

## Impact

Affects worker admission accounting, local status output, job-dispatch documentation, and worker unit tests. No server API or database changes are required.
