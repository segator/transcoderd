# Tasks

## 1. Controlled toolchains and compatibility checks

- [x] 1.1 Add guarded Linux SVT version/hash override plumbing for isolated 4.2.0/4.3.0 builds without advancing the default builder; verify that the expected old entry is replaced once, drift fails, and unrelated FFmpeg/dependency pins are unchanged.
- [x] 1.2 Build both Linux variants with identical surrounding inputs; retain build manifests, binary hashes, version/configuration output and successful 10-bit preset-4/CRF25 encode/probe/full-decode logs identifying the actual linked SVT versions.
- [x] 1.3 Prepare the matching Windows SVT 4.3.0 archive pin and toolchain record in an isolated build; extend native bundle checks and verify the existing encoder list plus the new SVT banner, 10-bit probe and strict decode scenarios. Keep the distributed default pending evidence review.

## 2. Reproducible offline version comparison

- [x] 2.1 Add scripts/av1_svt_versions.py with explicit baseline, candidate and metric tools plus a scene manifest; verify the generated baseline/candidate recipes use preset 4, CRF25, 10-bit output and the current width filter with no added tune/lp overrides.
- [x] 2.2 Implement build/input/model fingerprints and safe resume records; add focused tests demonstrating that replacing a binary or reference at the same path invalidates cached measurements and that identical paths/builds cannot silently serve as both versions.
- [x] 2.3 Implement exact frame/dimension/rate checks, every-frame VMAF mean/p5/p1/minimum/temporal windows and SSIM/PSNR using one fixed analysis toolchain; test rejection of missing/non-finite scores and mismatched or duplicated frame timelines.
- [x] 2.4 Implement alternating sequential repetitions after warm-up, median/dispersion/FPS, video payload versus container bytes and decoded-content checks before score reuse; verify a small real run records all commands, timings and output identities. Document the invocation and measurement boundaries in the comparison report.

## 3. Film-scene evidence and review

- [x] 3.1 Verify the three retained 24-second FFV1 references, extraction provenance, SDR metadata, hashes and frame counts; create an independent lab manifest and confirm originals are only read. Add a grain/motion/dark stress excerpt if available references lack that coverage.
- [x] 3.2 Run both versions at preset 4 / CRF25 with at least three uncontended sequential timings per scene and every-frame quality checks; deliver raw JSON, per-frame scores, strict decode logs and CSV summaries. Mark unavailable clean timings as unresolved rather than reporting a speedup.
- [x] 3.3 Produce matched source/4.2.0/4.3.0 frames at representative and worst-delta indices; review grain, dark detail, motion and banding. When quality shifts, measure adjacent CRFs on both versions and deliver separately labelled quality-matched results without extrapolation.
- [x] 3.4 Write docs/av1-svt-4-3-comparison.md and compact docs/benchmarks evidence, reporting each scene's time/dispersion, video bytes, quality deltas and visual findings. Record a supported promotion or hold decision; do not turn the upstream 10–11% claim or short clips into a whole-film guarantee.

## 4. Candidate promotion and integrated validation

- [x] 4.1 Only after favorable compatibility and scene review, advance the Linux builder tag and Windows version/provenance together; update encoding/configuration references and verify CPU preset 4 / CRF25 and GPU p7 / CQ38 remain unchanged. If evidence is insufficient, keep this promotion task open and retain 4.2.0.
- [x] 4.2 Run the lab's focused Python tests and relevant Go tests for worker/config, worker/step and the Mage build package; verify Linux candidate image gates and the native Windows bundle checks plus CPU worker E2E, recording unsupported/unexecuted platform checks explicitly.
- [x] 4.3 Compare delivered behavior with the distribution delta and run openspec validate --all --strict (or npx -y @fission-ai/openspec@latest validate --all --strict); verify all changed scenarios and record material existing discrepancies in openspec/BASELINE.md rather than hiding them.

## Workflow follow-up

- Publish the validated versioned builder through the normal authorized release process before workers consume it; retain the recorded 4.2.0 rollback identity.
- Archive only when the delivered integration and required validation are complete, synchronizing the distribution delta into the existing main capability.
