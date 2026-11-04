# Design

## Context
The existing runtime heartbeat is stored as JSON and returned by the dashboard API. The independent frontend must continue using that API only. Encoder slots measure configured capacity rather than hardware utilization.

## Goals / Non-Goals
Provide measured resource usage and useful worker drill-down without blocking scheduling/heartbeats or introducing a monitoring database. Do not attribute shared host/device usage exclusively to an encode or aggregate duplicate host readings across worker profiles.

## Decisions
- Add optional resources to WorkerRuntime. A worker-owned background sampler reads Linux /proc counters or Windows GetSystemTimes/GlobalMemoryStatusEx and NVIDIA nvidia-smi XML, preserving missing values as null. Windows sampling stays on one OS thread; multi-group systems explicitly report the current processor-group CPU scope/count.
- Sample every five seconds; bound GPU commands to two seconds and output to two MiB. Failures replace unavailable readings, never preserve an old GPU snapshot as live. CPU utilization requires two valid counters; idle zero is distinct from unavailable. Stop sampling with the worker.
- Read NVIDIA XML for GPU, encoder and decoder utilization, framebuffer memory, temperature and power, supporting legacy and current power tag layouts. Publish only selected metrics, never process lists or full command output. Per-device UUID/PCI identity and generic availability states survive partial N/A fields.
- Store current samples through the existing JSON column; add no migrations. Frontend overview links select a worker via #workers?worker=...; details retain capacity controls and show CPU/RAM and per-device GPU/NVENC plots. History is limited to 120 samples/ten minutes in page memory, never browser persistent storage or API caches. Duplicate timestamps do not create samples, and unavailable/stale readings do not draw fabricated zeroes.
- Metrics describe OS-visible host memory/CPU and visible NVIDIA devices, including other processes. Summary GPU values use the busiest reported device and say so for multiple devices. Resource freshness is checked independently of worker presence with a twenty-second threshold.

## Risks / Trade-offs
Container /proc may expose host-wide resources rather than quotas; scope is labeled. Windows CPU groups are labeled rather than presented as entire-host utilization on large systems. Missing drivers, unsupported sensors, older workers and failed commands must produce explicit unavailable states. UI refresh must preserve unsaved forms and selected-worker navigation.
