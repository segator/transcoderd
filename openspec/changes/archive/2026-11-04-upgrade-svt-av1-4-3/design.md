# Design

## Context

See proposal.md for motivation. `worker/config/video.go`, `worker/step/ffmpegStep.go`, the current media-encoding spec and `docs/encoding-profiles.md` agree on CPU preset 4 / CRF25, `yuv420p10le`, FFmpeg threading 0 and `scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease`. The older preset comparison's CRF23 describes historical settings.

Linux delegates compilation to ffmpeg-build-script tag v9.0.10, whose `src/10-versions.sh` pins SVT 4.2.0. Windows pins the same archive directly in `scripts/windows/svt.sh` and names it in `ffmpeg.sh`'s toolchain record. Existing Linux capability checks only list encoders, so an unchanged library can pass them.

The current lab scripts compare presets/encoders through one FFmpeg path. Their cache keys do not fingerprint replacement binaries at the same path; their hard-coded NVENC calibration cannot answer a library-version comparison.

## Goals / Non-Goals

**Goals:** Attribute observed changes to the SVT version; distinguish fixed-profile results from quality-matched results; provide repeatable evidence and compatible Linux/Windows bundles.

**Non-Goals:** Changing CPU/GPU quality defaults, adding worker metric passes or retries, ranking GPU versus CPU, establishing HDR quality, or extrapolating short clips to whole-film performance.

## Decisions

### 1. Change one dependency in matched builds

Keep the existing FFmpeg build script, FFmpeg 9.0.2, compiler, base image, build flags and all unrelated libraries. Use a guarded, repository-owned override of the SVT version/checksum entry after checking that the expected 4.2.0 entry exists exactly once. Fail on upstream layout drift. Do not upgrade the whole build-script release to obtain SVT.

The official candidate archive is:

- URL: https://gitlab.com/AOMediaCodec/SVT-AV1/-/archive/v4.3.0/SVT-AV1-v4.3.0.tar.gz
- SHA-256 observed from an HTTPS download on 2026-11-04: `95ce8fc2381ba707e65ac8b9b2e165915bfbe9cd4d2f7b0f58d64ce31bd98f6d`.
- Baseline archive SHA-256: `c7b13c4a84bd3751aa35fcc72be13e6875467e7c2216879251a486e5b1e4e740`.

Reverify the candidate when implementing. Build both variants through the same graph; a historical local binary is useful for inspection, but cannot alone prove compiler/dependency equivalence. Keep alternate-version selection local to benchmark build plumbing rather than expose it as worker configuration. Name the candidate builder distinctly, for example `builder-ffmpeg-9.0.2-av1-svt-4.3.0`.

### 2. Prove the linked version by exercising it

Retain existing FFmpeg/encoder-list gates and add a small CPU encode at preset 4 / CRF25 with 10-bit input, capturing SVT's actual version banner. Probe for AV1 10-bit output and strictly decode every frame. A generated toolchain text file alone cannot validate the linked library. Extend native Windows bundle validation in `integration/windows_bundle_test.go`; cross-compilation does not establish native execution.

### 3. Reuse retained lossless excerpts before extracting more media

Local inspection found retained FFV1 files in the original checkout:

- `build/av1-lab/pause-resume/reference-24s.mkv`
- `build/av1-lab/pause-resume/reference-late-24s.mkv`
- `build/av1-lab/pause-resume/reference-end-24s.mkv`

The first probes as 1920x800, 24 fps, bt709 SDR, 24 seconds and 576 decoded frames. It is 8-bit `yuv420p`; feed identical 10-bit conversions to both encoders without claiming restored source detail. Verify the other files and their original extraction commands/input hashes before using them. Copy reusable inputs into the independent lab or open them read-only; write all results under this worktree's ignored `build/av1-lab/svt-4-3/`.

Begin with these three excerpts. If they lack enough grain/motion/dark-scene variation, extract available historical intervals such as dark drama (1067 s), classic film (6027 s) and dialogue (2979 s), or the documented grain/action scene (5400 s), retaining source identity and exact decoded-frame boundaries. Previous multiclip temporary videos were removed; reports alone cannot substitute for actual reference files. Preserve originals and never enqueue or upload benchmark outputs.

### 4. Measure fixed profile before calibrating quality

Add `scripts/av1_svt_versions.py`, reusing safe helpers from `av1_lab.py`, with explicit baseline/candidate binaries, a separate fixed metric binary/probe, scene manifest, output directory and repeat count.

For each excerpt, encode the identical frames with the worker's scale filter, pixel format, preset 4, CRF25 and applicable timestamp/mux options. Exclude audio/subtitles and label measurements as video-only. Do not introduce `tune`, film-grain, tiling or `lp` overrides: those would change the comparison. A 24-second excerpt resets encoder history and therefore remains a scene test.

Run one unreported warm-up per build, then at least three sequential timed encodes per build, alternating baseline/candidate order between repetitions. Exclude counting, quality analysis and strict decode time. Record each wall time, median, dispersion, effective FPS, CPU/memory environment and competing workload observations. Do not pause production workers to obtain clean timings; use an idle host/window. Mark contended results unsuitable for speed claims and rerun later.

Compute video packet bytes separately from container bytes. Require exact equality of reference, decoded output and scored frame counts, matching dimensions/rate and strict full decode. Score every aligned frame with one metric binary/model and explicit common `yuv420p` conversion; record VMAF v0.6.1 mean, nearest-rank p5/p1, minimum and worst temporal windows plus SSIM/PSNR. Reject non-finite scores or silent frame duplication/truncation. Record metric commands and formats so 8-bit metrics are not confused with 10-bit fidelity evidence.

Fixed-profile results must show time, byte and quality deltas together. If candidate quality falls, test CRFs around 25 on both builds to bracket the baseline quality; report only measured matches and clearly labelled in-range interpolation. Do not extrapolate or hide a p5/p1 regression behind a matched mean. No numeric tolerance is treated as user approval of visual loss.

### 5. Keep evidence attributable and reviewable

Fingerprint binary content, build recipe/flags, library version, source/reference content, frame selection, all command vectors, model and metric binary. Include these in the cache identity; refuse reuse when any differ. Score each retained output, and verify repeated outputs by decoded-frame content before reusing scores. Similar file lengths do not establish identical frames.

Retain JSON/CSV results, logs and representative source/baseline/candidate frames at matching frame indices, including the worst quality deltas. Preserve raw per-frame arrays in temporal order before calculating sorted percentiles. Visual review covers texture/grain, motion, banding and dark detail; objective metrics cannot establish universal perceptual equivalence.

## Risks / Trade-offs

- [Build variation masks library effects] → Rebuild both versions with identical pinned surroundings and record provenance; report historical-binary checks separately.
- [Contention overwhelms a claimed 10–11% gain] → Alternate repeated runs, record dispersion, and reject busy-host timing conclusions.
- [Short excerpts miss long lookahead or content effects] → Limit conclusions to tested excerpts; a full-film follow-up is required for whole-film claims.
- [Metric quality improves while texture is lost] → Inspect matched frames and multiple metrics; report mixed outcomes rather than automatically promote.
- [Windows differs from Linux] → Require native version/10-bit validation and existing CPU E2E; Linux timings are Linux evidence only.
- [Source clips are unavailable] → Use verified retained references first and document missing coverage rather than replace them with synthetic footage.

## Migration Plan

1. Preserve the 4.2.0 reference recipe and builder identity. Produce isolated comparison binaries and complete the scene report before advancing the default builder.
2. Review compatibility, all speed/size/quality deltas and visual samples. If evidence is negative or incomplete, keep 4.2.0 as the distributed default and leave the change incomplete with findings recorded.
3. After a favorable review, update Linux builder tag and Windows pins/provenance together, validate encoder gates, worker tests and native CPU E2E, then publish the new builder through the normal release process when authorized.
4. Roll back using the recorded 4.2.0 recipe/builder and matching Windows bundle. Keep the current encoding defaults and previous evidence intact.

## Sources

- Official release notes: https://gitlab.com/AOMediaCodec/SVT-AV1/-/raw/v4.3.0/CHANGELOG.md
- Baseline dependency pins: https://github.com/markus-perl/ffmpeg-build-script/blob/v9.0.10/src/10-versions.sh
