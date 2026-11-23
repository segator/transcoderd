# Proposal

## Why

A zero frame-rate denominator causes an integer divide-by-zero panic and terminates the worker, interrupting unrelated jobs.

## What Changes

- Reject zero denominators and use the existing 24 fps normalization fallback.
- Add parser and normalization regression tests.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-encoding`: Define the fallback for an invalid frame-rate denominator.

## Impact

Worker FFprobe normalization, its tests, and the media encoding specification.
