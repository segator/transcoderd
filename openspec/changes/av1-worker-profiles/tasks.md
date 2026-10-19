# Tasks

## 1. Encoder configuration

- [x] 1.1 Register AV1 NVENC as the default codec and add encoder-specific quality, preset, and concurrency settings; verify CLI/YAML default and override tests.
- [x] 1.2 Validate supported video modes, compatible overrides, and positive concurrency limits before worker startup; verify invalid configurations fail before the initial server ping.
- [x] 1.3 Update the worker configuration reference and example for GPU default, CPU SVT-AV1, and x265 rollback; verify documented settings match the registered flags.

## 2. FFmpeg profiles and output verification

- [x] 2.1 Generate structured FFmpeg arguments for exact NVENC p7/UHQ/CQ34, SVT preset3/CRF23, and x265 rollback profiles while retaining stream mapping and scaling; verify table-driven command tests.
- [x] 2.2 Record actual encoded MKV bytes before verification and reject outputs larger than the source; verify size and duration checks with focused tests.
- [x] 2.3 Run real local GPU and CPU AV1 encodes from a representative fixture with the generated arguments, then probe and decode both outputs; verify codecs, bit depth, audio, subtitle mapping, and duration.

## 3. Bounded parallel jobs

- [x] 3.1 Apply the active-job limit together with pause and time-window admission; verify limit and time-window tests.
- [x] 3.2 Run up to the configured number of independent FFmpeg jobs concurrently without sharing job state; verify overlap, queuing at the limit, and per-job results in tests.
- [x] 3.3 Document concurrency configuration and limits for GPU and CPU workers; verify examples match actual flags and queue behavior.

## 4. End-to-end and review

- [x] 4.1 Update the Docker E2E fixture path to exercise AV1 CPU without a GPU and verify the resulting MKV and completion event.
- [x] 4.2 Exercise the worker container's AV1 NVENC path on the RTX 5080, including GPU access and output probing; verify the exact profile works inside the image.
- [x] 4.3 Run relevant Go tests, race tests, build, `openspec validate --all --strict`, and a changed-file credential scan; record concrete results and address failures.
- [ ] 4.4 Open a PR against `codex/av1-benchmark-lab` with the implementation, specification, migration guidance, and verified test results; attach the PR to this task.

## Workflow follow-up

- Archive the OpenSpec change once the implementation is reviewed and merged so the main specs describe shipped behavior.
