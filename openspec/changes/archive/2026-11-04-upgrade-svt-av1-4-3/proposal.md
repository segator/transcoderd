# Proposal

## Why

SVT-AV1 4.3.0 was released on 2026-11-01 and advertises approximately 10–11% lower multithreaded runtime at similar quality. TranscoderD currently bundles 4.2.0; an isolated comparison using its actual CPU profile is needed before attributing a speed or compression improvement to the upgrade.

## What Changes

- Prepare a checksum-pinned SVT-AV1 4.3.0 toolchain for Linux and native Windows while retaining FFmpeg 9.0.2 and the other pinned dependencies.
- Compare controlled 4.2.0 and 4.3.0 builds on identical SDR film excerpts with the current worker recipe: preset 4, CRF25, 10-bit 4:2:0 and the existing width filter.
- Add an offline version comparison runner with sequential repeated timings, video payload sizes, every-frame VMAF, SSIM/PSNR, strict decode checks, and build/input fingerprints.
- Report fixed-profile changes first; use an additional measured CRF ladder when needed to distinguish speed/size improvements from reduced quality. Retain commands, frame scores and visual review samples.
- Advance the versioned builder and bundle provenance only after compatibility and scene evidence have been reviewed; retain 4.2.0 as the comparison/rollback reference.

## Capabilities

### New Capabilities

None. The comparison is offline engineering tooling, not a worker quality prepass.

### Modified Capabilities

- `distribution`: Specify SVT-AV1 4.3.0 behind bundled FFmpeg's CPU AV1 encoder on Linux and Windows, including an executable version/10-bit encode check.

The existing `media-encoding` defaults remain preset 4 / CRF25 for CPU and NVENC p7 / CQ38 for GPU. No API, configuration or job execution contract change is proposed.

## Impact

- `magefiles/magefile.go`: focused SVT dependency override, a new builder tag and actual encoder version/encode gates.
- `scripts/windows/svt.sh`, `scripts/windows/ffmpeg.sh` and bundle checks: matching version/hash and toolchain provenance.
- `scripts/av1_svt_versions.py` and focused lab tests: a two-build comparison rather than the existing NVENC/preset calibration loop.
- `docs/av1-svt-4-3-comparison.md`, small evidence summaries under `docs/benchmarks/`, `docs/encoding-profiles.md` and `openspec/CONFIGURATION.md`: reproducibility, observed deltas and version provenance. Large media stays under this worktree's ignored `build/` directory.
- Linux builder publication and Windows release validation remain required before distribution. This proposal neither publishes images nor changes a live worker.
