# Design

## Context

See proposal.md. The dashboard currently edits a single persisted limit, and each worker has one FFmpeg queue with a dynamic concurrency counter. Encoder configuration is shared by all jobs. Approved defaults are CPU SVT-AV1 preset 4/CRF25 and GPU AV1 NVENC p7/CQ38.

## Goals / Non-Goals

**Goals:** execute both encoder classes concurrently in one process; tune each independently without interruption; keep accepted/restored work and legacy configurations predictable.

**Non-Goals:** changing quality defaults, automatic hardware fallback, GPU device selection, hardware utilization inference, and changes to dashboard authentication.

## Decisions

- Add nullable `cpu_jobs` and `gpu_jobs` columns to worker_settings. A pair is atomic, both present or both absent; its sum supplies the legacy aggregate. Single-limit writes clear the pair. Reject negative values, incomplete/mixed requests, and aggregate integer overflow before persistence.
- New clients declare independent-encoder support when loading settings. If a legacy client reads a stored pair, return aggregate zero so it drains instead of incorrectly applying the sum to its only encoder. Existing single-limit reads remain byte-compatible.
- Extend the current concurrency controller with independently counted CPU/GPU slots and immutable startup preferred encoder. A shared bounded preparation/verification/upload pipeline feeds available slots, favoring the preferred encoder when both are free. Admission uses the sum of desired positive slots plus two buffer jobs. Decreasing a positive target drains running work. Disabling one class prevents new unassigned jobs using it; jobs already pinned to it may resume using its retained positive drain target. Disabling both closes admission while accepted work drains using retained targets. Local legacy controls retain their meaning and positive values return to the startup encoder only.
- Use the existing selected profile for its encoder class. The other class uses approved defaults with shared audio/thread settings; do not copy hardware-specific extraArgs or quality parameters across encoder classes. Optional YAML cpuFFmpegConfig/gpuFFmpegConfig blocks permit explicit class-specific profiles and are validated at startup.
- Pin the selected class and a copied FFmpeg configuration in each job context before publishing FFMPEG/started. With only one class enabled, pin it before publishing Job/started, preserving that choice even when an in-flight assignment arrives after a pause. Resumed encodes use that snapshot and class; older contexts without it select an available class. Phase messages identify class/encoder for the job table. Do not mutate global FFmpeg configs during execution.
- Keep aggregate telemetry for compatibility and add a capability version, configured/active/effective counts per class, and encoder names. Worker cards edit CPU and GPU together and show pending acknowledgement. Each card remains keyed by stable worker name across refreshes.

## Risks / Trade-offs

- [GPU slots require working NVIDIA runtime/hardware] → A CPU-only deployment starts with GPU zero; explicitly enabled GPU work uses NVENC and reports normal encoder failures if the runtime is unavailable. Never silently switch quality or backend.
- [Zero limits could strand restored work] → Persist profile snapshots and preserve per-class drain targets, with minimum one for a restored pinned class.
- [Mixed legacy and modern binaries] → Server-first rollout, capability-aware reads, explicit upgrade labels, and safe legacy pause behavior.
- [Concurrent changes could produce half a pair] → One SQL upsert and one settings mutex; validate and commit before applying local controls.

## Migration Plan

Deploy the additive server migration first, then upgraded workers. Existing settings rows have no pair and preserve their current encoder/count. Operators then save both limits from the dashboard. Before downgrading a dual-capable worker, drain all accepted work because older binaries cannot honor profile snapshots. Replace dual overrides with suitable legacy limits, or let capability-aware reads pause a legacy worker safely.
