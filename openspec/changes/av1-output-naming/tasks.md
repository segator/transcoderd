# Tasks

## 1. Server-side output probing

- [x] 1.1 Add a server helper that probes the uploaded target's primary video codec and maps `av1`, `hevc`, and `h264` to stable filename labels; verify unit tests for each mapping and probe failure.
- [x] 1.2 Document that final output names follow the codec detected in the uploaded file and explain the HEVC-to-`x265` convention; verify the docs agree with the codec mapping tests.

## 2. Final destination naming and persistence

- [x] 2.1 Derive the final filename from the source basename and probed output codec, recognizing `x264`, `h264`, `x265`, `h265`, and `hevc`; verify scheduler tests cover H.264-to-AV1, HEVC-to-AV1, AV1-to-HEVC, unlabelled inputs, audio labels, and source-path collisions.
- [x] 2.2 Keep each upload in a job-specific staging path, then finalize with no-clobber filesystem semantics and persist the final `target_path` and target size before recording completion; verify tests prove existing files are never overwritten and filename conflicts select `_encoded`, `_encoded-2`, or the next free suffix without leaving the job incomplete.
- [x] 2.3 Ensure completion finalization is idempotent on retries and that `deleteOnComplete` runs only after successful probe, rename, and persistence; verify scheduler tests cover retries and both deletion setting values.
- [x] 2.4 Verify token-boundary matching preserves codec-like text inside ordinary title words by adding negative filename tests and running `go test ./server/scheduler/...`.
- [x] 2.5 Place new upload staging files on the destination filesystem and migrate legacy root-staged uploads across filesystems before finalization; verify cross-filesystem finalization, no-overwrite behavior, and idempotent retries.

## 3. Contract validation

- [x] 3.1 Run relevant server and scheduler tests, then run `openspec validate --all --strict`; resolve any failures before completing the change.
- [x] 3.2 Verify the real PostgreSQL integration stores the probed AV1 final path and size before the completion event; verify HEVC and H.264 label mapping with scheduler unit tests.
- [x] 3.3 Reproduce an overlay-to-bind-mount staging layout and verify the server can finalize it without `EXDEV`, including after a retry.

## Workflow follow-up

- Archive the change after implementation and review are complete.
