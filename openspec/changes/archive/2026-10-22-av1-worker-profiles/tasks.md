# Tasks

## 1. Encoder configuration

- [x] 1.1 Register AV1 NVENC as the default codec and add encoder-specific quality, preset, and concurrency settings; verify CLI/YAML default and override tests.
- [x] 1.2 Validate supported video modes, compatible overrides, and positive concurrency limits before worker startup; verify invalid configurations fail before the initial server ping.
- [x] 1.3 Update the worker configuration reference and example for GPU default, CPU SVT-AV1, and x265 rollback; verify documented settings match the registered flags.

## 2. FFmpeg profiles and output verification

- [x] 2.1 Generate structured FFmpeg arguments for exact NVENC p7/UHQ/CQ34, SVT preset4/CRF23, and x265 rollback profiles while retaining stream mapping and scaling; verify table-driven command tests.
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
- [x] 4.4 Open a PR against `codex/av1-benchmark-lab` with the implementation, specification, migration guidance, and verified test results; attach the PR to this task.

## Workflow follow-up

- Archive the OpenSpec change once the implementation is reviewed and merged so the main specs describe shipped behavior.

## 5. Full-pipeline deployment validation

- [x] 5.1 Preserve the container's NVIDIA library search path in the FFmpeg child process and verify a GPU job through the Docker E2E server and worker.
- [x] 5.2 Document separate GPU and CPU worker limits and retain the previous x265 command as a legacy benchmark reference.
- [x] 5.3 Run a complete local movie through the server and GPU worker, then verify its output, size, streams, duration, and event history.
- [x] 5.4 Rerun Go tests, lint, OpenSpec validation, and changed-file credential scan; update PR #126 with results.

## 6. Deployment and CPU trade-offs

- [x] 6.1 Document Linux, Unraid, and WSL NVIDIA worker prerequisites, separate CPU/GPU worker configuration, image build dependencies, and every production encoder option.
- [x] 6.2 Compare SVT presets 3–6 with the production NVENC profile at similar scene quality; record measured time/size/quality and recommend selectable CPU options.

## 7. Weekly job admission

- [x] 7.1 Add CLI/YAML weekly windows and timezone settings with startup validation, retain legacy-window compatibility, and reject conflicting settings.
- [x] 7.2 Apply weekly windows before new job requests without stopping accepted work; test exact boundaries, multiple windows, weekends, overnight windows, timezone offsets, DST, pause, and concurrency limits.
- [x] 7.3 Document cutoff-only and weekday/weekend examples for both workers, explain job overruns and tariff/calendar limits, and express the measured CPU/GPU comparison in minutes with projection limits.
- [x] 7.4 Run Go and race tests, lint, strict OpenSpec validation, review the diff for credentials, and update PR #126 with the results.

## 8. Whole-film benchmarks and simpler defaults

- [x] 8.1 Extend full-film E2E controls/reporting and a resumable local runner to retain exact settings, phase timings, CPU/memory telemetry, and completed outputs safely.
- [x] 8.2 Compare CPU presets 6, 4, and 3 against fixed GPU on complete 60-second source clips from H.264 film, HEVC concert, and difficult-scene material; score every clip frame and retain repeated isolated timings. Treat results as provisional rather than whole-film claims.
- [x] 8.3 Make omitted encode concurrency inherit maxActiveJobs, select provisional CPU preset 4 from the completed short-scene evidence, and verify explicit overrides/default-only worker selection.
- [x] 8.4 Support simple daily CLI schedule shorthand and document portable CLI-only Docker commands with the revised 22:00-10:00 Europe/Madrid schedule.
- [x] 8.5 Publish completed measurements with clear quality/time/resource boundaries, run relevant tests/builds/spec validation and credential review, and update PR #126.
