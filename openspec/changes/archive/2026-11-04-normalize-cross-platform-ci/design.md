# Design

## Context

The repository requires a check named `ci`. Both Linux and Windows cross-builds connect to the same observed Kubernetes Dagger Engine. In run 41876078723 all media dependencies hit cache, but production NSIS packaging took 14m47s and two concurrent full update fixtures took another 14m43s. The uploaded artifact was 3,660,789,366 bytes. Engine storage and CPU limits are infrastructure-owned; CI receives a compiler budget of 16 as requested by its owner.

## Goals / Non-Goals

**Goals:** Consolidate cross-compilation orchestration, prevent unrelated dependency invalidation, use bounded parallel builds, reduce measured packaging costs and preserve the complete release validation path.

**Non-Goals:** Change encoder output, remove OCR languages, change installer payload/runtime requirements, weaken update tests, migrate infrastructure or delete live maintenance/recovery workflows.

## Decisions

1. Move the Windows build steps into `ci` and native Windows execution continues in `windows-unit` and `windows-native`. Preserve the existing required check name. Put lint in `ci` and remove its separate workflow. Debug dispatch skips ordinary compilation and continues to reuse earlier trusted fixtures. PR concurrency cancels only superseded runs for that PR; main/release dispatch runs remain independent.
2. Build AOM, SVT, x265, FDK and NVCodec from a common MinGW base. The four substantial C/C++ builds divide the total budget; FFmpeg receives the full budget after their installed prefixes and licenses are merged. Default to a budget of 16, matching the owner's Engine, with `WINDOWS_BUILD_JOBS` as an explicit positive-integer override for smaller/larger Engines. A bounded Go errgroup limits active dependency evaluations, including budgets below four, so concurrent compiler counts cannot exceed the budget. Avoid detecting CPUs on the GitHub runner: compilation runs on the connected Engine. A changed stage script affects only that dependency and the dependent FFmpeg recipe.
3. Benchmark NSIS options using the real pinned media/OCR payload before selecting settings. Preserve a standalone compressed installer and all update fixtures. Use zlib level 3 with default datablock deduplication: the same 385-file payload took 34.75s rather than 500.41s and grew by 3.88%. Disabling deduplication took 33.32s and grew the output more, so retain the default. Verify identical extracted digests. The portable ZIP also uses level 3. Assemble stable media/OCR/UI/prerequisite directories before application binaries, and share the worker binary recipe between portable and installer outputs. Avoid a new extraction dependency or smaller test-only payload. Record measured time and size trade-offs separately from GitHub timings.
4. Retain content-addressed dependency caching and Go module/build cache mounts. CI evaluates image graphs with `docker:check` instead of serializing/loading large archives that Dagger E2E never consumes; local `docker:all` and Linux release exports remain. Explicit Linux-release and Windows-artifact allowlists prevent accidental uploads across platforms after consolidation. Do not add an unverified remote-cache service. Remove only code/actions/fixtures proven unused; retain scheduled maintenance, Dependabot auto-merge and isolated Windows debug support.

## Risks / Trade-offs

- Concurrent encoders could oversubscribe a 16-core Engine → divide the total compiler budget among independent branches; compile FFmpeg only after them.
- Faster packaging can grow installers → compare bytes and extraction integrity using the same payload, document the result and retain identical installed files.
- Workflow consolidation could bypass native/release gates → validate the job graph and release-source guards; preserve publication only after native validation.
- Cold and shared Engine timing varies → report local measurements and actual Actions timings separately; do not promise a fixed PR duration.

## Migration Plan

Update the existing PR, run focused tooling tests plus real cross-compilation/packaging and strict OpenSpec validation, and inspect the new Actions run. Reverting the pipeline commit restores the old orchestration and packaging options without changing installed-worker compatibility.
