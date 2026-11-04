# Proposal

## Why

The operator clarified that each individual worker must support independent CPU and GPU concurrency simultaneously. The current single-encoder limit cannot express two CPU plus three GPU encodes within one process.

## What Changes

- Persist an atomic per-worker pair of non-negative CPU/GPU limits and synchronize it at runtime.
- Dispatch actual encoding work across independent CPU and NVENC slots, preserving approved encoder profiles and per-job profile snapshots across restart.
- Display and edit both desired limits, observed activity, and pending synchronization in each dashboard worker card.
- Preserve existing single-limit startup and local control behavior for deployments without a pair. Legacy workers pause new admission when a dual configuration exists, avoiding accidental over-assignment to their only encoder.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-dispatch`: independent CPU/GPU execution and admission limits, including draining and restoration.
- `media-encoding`: separate encoder profiles and immutable per-job profile selection.
- `operations-dashboard`: atomic two-limit controls and split runtime metrics.
- `persistence`: durable nullable CPU/GPU override pair and safe legacy reads.

## Impact

Worker execution/concurrency, job contexts, encoder config selection, settings API/client/repository and additive migration, telemetry, dashboard controls, protocol/configuration/operational docs, and unit/PostgreSQL/browser verification. Existing authentication remains unchanged.
