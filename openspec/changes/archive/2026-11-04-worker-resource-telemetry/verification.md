# Verification — 2026-11-04

## Contract and implementation

- Optional `WorkerRuntime.Resources` travels through the existing heartbeat and runtime JSON column. `TestHardwareTelemetryHeartbeatToDashboard` verifies worker serialization, HTTP event ingestion and authenticated dashboard readback, including genuine idle zero, unavailable GPU core readings with valid encoder readings and older workers without resources.
- Worker collection starts/stops with execution. Tests cover CPU warm-up, deltas/reset/scope, Linux CPU guest counters and MemAvailable, independent snapshots, cancellation, subprocess deadlines and the output cap during `io.Copy`. NVIDIA XML fixtures cover multiple devices, partial/invalid sensors and both power layouts.
- Overview rows link to an exact selected worker. CPU/RAM and per-device GPU/NVENC plots retain at most 120 samples/ten minutes in page memory. Unit checks cover zero versus unknown, stale/offline/clock mismatch, independent multi-device maxima, URL names, deduplication/expiry, missing-value gaps and HTML escaping.

## Automated checks

- Full Linux `go test -race ./...`: pass.
- golangci-lint v2.11.4 for Linux and Windows: 0 issues.
- `node --test frontend/web/*_test.cjs`: 12 tests pass, including resource rendering/history and PWA cache privacy.
- Native Windows resource test executable: pass. Native opt-in hardware sampling also passes, exercising Windows CPU/memory APIs and actual NVIDIA XML rather than cross-compilation alone.
- `openspec validate --all --strict`: 25 items pass before archive; main specs validated again after synchronization.
- Chromium resource test: actual CPU/GPU/NVENC overview values, keyboard drill-down, selected card, live SVG charts, preserved unsaved CPU draft across refreshes, return to all workers, older-worker unavailable state, no page errors and no mobile overflow at 390px.
- Chromium English UI checks: login/errors, all four views, worker controls, media API, preserved filenames, desktop/mobile layout, manifest and offline page pass.
- Chromium PWA lifecycle checks: installable manifest, public asset cache including `resources.js`, no private API/media/HTML cache, offline reconnection, authoritative logout and two-version explicit-reload update behavior pass.

## Live sampler evidence and local preview

Read-only Linux sampling reported 24 logical processors, CPU 11.8%, RAM 1.7/20.97 billion bytes and an RTX 5080 with GPU 16%, NVENC 78%, 49.1°C and 98.65W. Native Windows sampling reported 24 logical processors, CPU 97.7%, RAM 31,080,558,592 / 68,086,493,184 bytes and the RTX 5080 with GPU 19%, NVENC 95%, 51.13°C and 102.79W. These are observations at different times and OS-visible scopes, not benchmarks or job-attributed measurements. No extra encode workload was started.

The independent frontend runs at `http://localhost:18080/dashboard/` against the API-only preview at `http://localhost:18081`. Synthetic job/worker fixtures remain distinct from `local-hardware-preview`, a paused observer publishing actual local resource samples with zero encode slots. Older synthetic workers correctly show unavailable hardware telemetry. The frontend has no direct hardware or media filesystem access. Preview binaries/helpers and screenshots stay outside the repository.

Prior PR head `bcba73f` completed Linux/E2E, Windows unit/build/native, PWA, lint and CodeQL checks successfully. This change requires its own fresh CI run after publication; prior CI results are not evidence for the new commit.
