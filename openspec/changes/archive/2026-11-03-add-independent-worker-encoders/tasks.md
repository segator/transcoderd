# Tasks

## 1. Atomic settings and compatibility

- [x] 1.1 Add paired settings validation, SQL migration/upserts, memory repository, capability-aware worker API/client and dashboard writes; verify isolation, invalid/mixed/incomplete inputs, overflow, legacy projection, and fresh PostgreSQL reads.

## 2. Actual independent encoding

- [x] 2.1 Implement independent CPU/GPU slot counters, live resizing/draining and bounded admission; verify simultaneous 2+3 execution, per-class disabling, global pause, restoration, and legacy behavior under race detection.
- [x] 2.2 Resolve separate validated profiles, pin copied profiles into job contexts before execution and use them in FFmpeg; verify CPU/NVENC command separation and context round trips without changing approved quality defaults.
- [x] 2.3 Synchronize paired settings and publish split telemetry; verify remote changes, legacy fallback, and restored/draining state.

## 3. Dashboard and operational references

- [x] 3.1 Replace per-worker single controls with atomic CPU/GPU controls and observed counters, split fleet metrics, pending and legacy states; verify browser editing, pause, isolation and responsive layouts.
- [x] 3.2 Update dashboard, protocol, configuration, deployment and readme references; verify documented payloads/defaults against tests and CLI behavior.

## 4. Integrated verification

- [x] 4.1 Run relevant and full race tests, PostgreSQL integration, lint, build, browser checks and strict OpenSpec validation; inspect all delivered delta scenarios before synchronization and archive.

## Workflow follow-up

- Synchronize the delivered specs and archive the change after verification.
