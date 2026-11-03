# Design

## Context

The desktop service currently supervises one worker process, and the worker's codec and encode limit are process-wide settings. The server already accepts independent worker names and limits, and assigns any queued job to the first eligible request.

## Goals / Non-Goals

**Goals:**
- Supervise one CPU worker and one GPU worker from the existing Windows service.
- Keep each process's configuration, workspace, control pipe, and persisted server limit independent.
- Present combined activity through the existing tray dashboard.
- Migrate the existing single-worker settings without losing the user's codec or concurrency.

**Non-Goals:**
- Route jobs by source format, quality, or user-selected preference.
- Change the server's queue assignment protocol.
- Support more than one CPU profile and one GPU profile in the dashboard.

## Decisions

- Run two standard worker child processes rather than changing FFmpeg step execution. Each child retains the existing codec-specific FFmpeg profile and concurrency behavior.
- Derive worker names as `<name>-cpu` and `<name>-gpu`; each worker independently requests jobs from the existing shared queue. Assignment remains opportunistic.
- Give each child a distinct Windows named pipe through an environment override, while preserving the existing default pipe for standalone workers.
- Store CPU and GPU codec/concurrency settings in service state. When loading legacy state, map NVENC to GPU and SVT-AV1 or x265 to CPU, retaining codec and limit.
- Aggregate both control statuses for the UI. Tag job rows with their profile; sum counts and progress from both children.
- Apply profile codec changes by draining both workers before restarting them. Change concurrency limits live through each worker's existing control pipe.

## Risks / Trade-offs

- Either profile may receive any queued job, so the CPU worker can encode a job that could have used the GPU → This matches current server assignment semantics and is shown in the UI.
- Two worker processes use more memory for duplicate worker runtimes → The user controls per-profile concurrency and can set an unused profile to zero.
- Derived worker names can exceed server limits → Validate the base name length after reserving the profile suffix.
- One child may restart while the other continues → Track child lifecycle independently and keep aggregate status available when either child is temporarily unavailable.

## Migration Plan

On first load of legacy service settings, map the saved codec to a CPU or GPU profile and preserve the saved worker limit. Persist the migrated shape atomically on the next settings save. Existing work directories remain intact; the new profile names create separate work subdirectories.

Rollback uses the existing installer rollback. Legacy settings remain readable by the new service, while an older binary can ignore the new fields only after configuration is backed up or removed.
