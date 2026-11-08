# Proposal

## Why

Concurrent SVT-AV1 encoding competes with software decoding for CPU. Benchmarks show that NVIDIA hardware decoding can improve throughput on the existing GPU worker.

## What Changes

- Use NVIDIA decoding for eligible GPU AV1 jobs, preserving existing resolution and 10-bit output policies.
- Retain software decoding for incompatible inputs and when hardware decoding cannot initialize.
- Measure hardware decoding for SVT independently without changing CPU defaults.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-encoding`: Specify hardware decoding eligibility and safe software fallback for GPU jobs.

## Impact

Worker FFmpeg command construction, media probing, tests, and operational documentation.
