# Tasks

## 1. Owned encoding process suspension

- [x] 1.1 Implement a command pause controller with Windows/Linux suspension, serialized starts and exit cleanup; verify real child-process progress stops/resumes, paused starts wait and cancellation terminates suspended processes.

## 2. Worker and settings integration

- [x] 2.1 Add validated finish/ram configuration, FFmpeg controller wiring, local mode control and status; verify worker pause/resume, mode changes and maintenance drain tests.
- [x] 2.2 Persist desktop mode and hot-apply it to both profiles without restarting encodes; verify settings round trips, generated configuration and Windows service tests/builds.
- [x] 2.3 Add settings selector, accessible tooltip and accurate paused notices/job labels; verify dashboard JavaScript behavior and document RAM/VRAM, shutdown and timeout boundaries.

## 3. Integration validation

- [x] 3.1 Run relevant Go tests with race detection, native Windows lifecycle checks, controlled encoder output comparisons, Windows cross-builds, dashboard checks and strict OpenSpec validation; record results and limitations.

## Workflow follow-up

- Archive the completed change and verify the synchronized main specs.
