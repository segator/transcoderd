# Proposal

## Why
Operators can see encode slots and job outcomes but cannot tell how busy CPU, GPU or the video encoder actually are. They need a concise fleet summary with direct access to useful worker resource detail.

## What Changes
- Collect read-only, bounded CPU/RAM and NVIDIA GPU telemetry on each worker and attach optional snapshots to existing heartbeats.
- Add compact CPU/GPU/encoder usage to overview worker rows with links to a selected worker in the Workers view.
- Show CPU/RAM history, per-GPU utilization/encoder history, VRAM, temperature, power and freshness in worker details, retaining existing capacity/pause controls.
- Distinguish missing/unsupported, warm-up and stale data from real zero; label host/device scope and keep history bounded and in memory only.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `events-and-progress`: optional host/device resource samples through worker presence events.
- `operations-dashboard`: compact utilization summaries and focused worker resource details.

## Impact
Worker sampler and platform readers, additive model fields, existing JSON telemetry persistence, frontend rendering/navigation, tests and operator documentation. No database migration, new service, frontend filesystem access or encode scheduling change.
