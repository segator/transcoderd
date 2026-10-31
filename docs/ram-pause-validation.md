# RAM pause validation — 2026-10-31

The desktop now separates **Stop after current / Resume queue** from **Pause encodes / Resume encodes**. Queue control lets active work finish; RAM control suspends active CPU/GPU encoding immediately without changing limits. Each preserves the other's state and individual holds. The desktop's **Pause mode** identifies the RAM suspension strategy. Downloads, verification, uploads and coordination remain responsive; queued encodes wait while either hold is active.

RAM and GPU VRAM remain allocated. Process/service exit, machine shutdown or reboot loses this state. Closing the tray alone does not stop the service. Server assignment timeout remains in force (default 24h from the last Job event). Maintenance temporarily resumes suspended encodes and drains all accepted work without changing the saved desktop pause. Durable/chunked resume is outside this change.

## Process and worker checks

Real owned child processes were exercised on Linux/WSL2 and native Windows, including repeated/idempotent pause and resume, stable output while suspended, cancellation while suspended, canceled starts, starts held until resume, concurrent start/exit transitions and an unregistered process continuing to run. Linux waits for the OS stop acknowledgement without reaping the child before reporting success; repeated race runs (ten per package) cover this boundary. Error tests verify refused transitions and rollback of a partially applied suspension. Worker tests exercise actual process suspension through runtime limits, per-job paused status, hot mode changes and maintenance resume with admission still closed.

Native Windows tests also cover the local mode command and desktop mode-only configuration: no pending restart/drain, retained credentials, saved pause and persisted mode. Windows worker and service binaries cross-build successfully. Tests use their own processes and temporary files; the installed service and its production jobs are not modified.

## Compressed-stream comparison

FFmpeg 9.0.2 encoded the same deterministic synthetic 320×180, 24 fps, eight-second input continuously and through the implemented pause controller. Each paused run suspended once for 0.5s after encoded-frame progress. Input was paced for the test window. Outputs were elementary streams (IVF for AV1, HEVC for x265), allowing exact byte comparisons without Matroska metadata noise.

The tested profiles use SVT preset 4 / CRF25, NVENC p7/UHQ / CQ38 / full-resolution multipass / four middle-reference B frames / QP0–100 / split disabled, and x265 medium / main10 / CRF21. CPU test thread pools are bounded to two; this is not a throughput benchmark.

| Platform | Encoder | Continuous and paused bytes | Exact compressed stream |
| --- | --- | ---: | --- |
| Linux/WSL2 | SVT-AV1 | 290,821 | Identical |
| Linux/WSL2 | AV1 NVENC | 311,876 | Identical |
| Linux/WSL2 | x265 | 314,612 | Identical |
| Native Windows | SVT-AV1 | 290,821 | Identical |
| Native Windows | AV1 NVENC | 311,876 | Identical |
| Native Windows | x265 | 314,614 | Identical |

Equality is within each platform/encoder pair. The Windows and Linux x265 streams differ from each other; cross-platform equality is not claimed. These are short implementation regression checks. They do not replace the earlier complete-film investigation or imply a new complete-film benchmark.

Reproduce on a machine with the project's FFmpeg and a compatible NVIDIA GPU:

```bash
TRANSCODERD_PAUSE_TEST_FFMPEG=/path/to/ffmpeg \
  go test -v -run '^TestRAMPauseFFmpegOutput$' ./helper/command
```

The encode comparison is opt-in and skips without that variable, so ordinary CI does not require a GPU.

## Application and browser checks

Application packages pass race tests using:

```bash
go test -race ./cmd/... ./helper/... ./model/... ./server/... ./update/... ./version/... ./worker/...
node scripts/windows/test-web-ui.cjs
```

`go test ./...` in this local checkout also discovers an existing ignored scratch benchmark under `build/windows-linux-comparison/compiler-bench` that imports Dagger outside its module; that broad command fails there. The explicit package command above covers all application packages without changing those unrelated local artifacts.

Command/worker lint and `go vet` pass. Strict OpenSpec validation passes. A Chromium test using the real embedded assets and a simulated desktop service verified the saved settings payload, keyboard-accessible tooltip, CPU/GPU paused badges, hidden stale speed/ETA, Resume and mobile layout. It is a browser interaction check, not an installed-service UI test.

Raw logs, test-only binaries, screenshots and the browser harness are retained in ignored `build/ram-pause-validation/`. No production installer/service replacement was performed.

## Individual control extension — 2026-10-31

The individual-control extension adds Pause/Resume to active encoding rows while retaining the general button. General Resume preserves individually paused encodes. Per-row status also identifies a general hold from a disabled/paused CPU or GPU profile, even when the other profile is active. Individual controls retain the encode slot and always suspend in RAM regardless of the configured general mode. Maintenance clears live individual holds, resumes accepted work and blocks individual mutations while draining. Local and authenticated browser controls accept job UUIDs, and the service resolves the owning profile from authoritative snapshots.

Additional evidence is retained in `build/ram-pause-validation/individual-*`: two real processes with independent progress, global/manual hold composition, unchanged worker slots and limits, native Windows profile routing and maintenance guards, authenticated HTTP/IPC controls, and real-browser row interactions. Encoded-stream comparison now exercises both general and individual pause against the same continuous reference for all three encoders on Linux and native Windows.

## Separate general controls — 2026-10-31

The current panel separates **Stop after current / Resume queue** from **Pause encodes / Resume encodes**. Admission and RAM preferences are saved independently. RAM resume preserves a stopped queue; queue resume preserves RAM suspension. Individual holds remain independent. Desktop strategy defaults/migrates to RAM, while standalone legacy zero-limit policy remains compatible.

Real child-process checks verify that stopping the queue allows active progress, RAM pause stops it immediately, both resume orders preserve the other hold, settings resolution preserves startup RAM gating, and maintenance releases live holds. Saved configuration tests cover all four queue/RAM combinations for CPU and GPU profiles. Browser interaction checks exercise both buttons and individual controls; evidence is in `build/ram-pause-validation/separate-*`.
