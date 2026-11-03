# Tasks

## 1. Settings and compatibility

- [x] 1.1 Add validated CPU/GPU profile settings and migrate legacy single-profile JSON; verify legacy and mixed settings load through the desktop configuration path.
- [x] 1.2 Generate one worker configuration per profile with independent codec, limit, name, and work directory; verify generated configs encode 1 CPU and 4 GPU jobs separately.

## 2. Windows service supervision

- [x] 2.1 Add per-worker named-pipe selection while retaining the standalone worker default; verify each worker can expose and address a distinct control pipe.
- [x] 2.2 Supervise both profile processes, persist limits per derived worker name, and drain/restart both safely for codec or connection changes; verify an inactive profile does not request jobs.
- [x] 2.3 Aggregate status, jobs, progress, pause, resume, and reconfiguration state across both workers; verify the aggregate reports both profiles and summed concurrency.

## 3. Dashboard and installer

- [x] 3.1 Replace the single encoder/concurrency setting with separate CPU and GPU profile controls and explain opportunistic queue assignment; verify each profile can be independently disabled.
- [x] 3.2 Identify the encoding profile on job cards and show aggregate encode capacity; verify mixed active work is distinguishable in the dashboard.
- [x] 3.3 Update Windows installation checks and deployment documentation for dual profiles; verify the installer still packages the dashboard and service.

## 4. Integration

- [x] 4.1 Build the Windows service and desktop app and run strict OpenSpec validation; resolve all compiler and spec errors.
