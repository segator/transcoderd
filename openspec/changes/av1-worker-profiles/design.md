# Design

## Context

See proposal.md. The worker currently generates one HEVC-shaped FFmpeg command string regardless of `videoCodec`; `command.NewCommandByString` reparses that string. Its encode queue has one consumer, and `maxActiveJobs` is a fixed value of two. The existing `parallelRunners` setting controls actions **within one job**, not simultaneous jobs. Download records source bytes, but FFmpeg does not populate target bytes before the size check. The upstream FFmpeg builder in PR #123 contains both NVENC and SVT-AV1.

## Goals / Non-Goals

**Goals:** Produce the approved NVENC AV1 video arguments exactly, offer a tested SVT-AV1 CPU option, preserve x265 rollback, safely bound simultaneous jobs, and verify output size before upload.

**Non-Goals:** Automatic per-title encoder/CQ selection, VMAF prepasses, CUDA decode or zero-copy conversion, split-frame encoding, multi-GPU scheduling, and automatic replacement of existing library files without the existing server workflow.

## Decisions

### Encoder selection and compatibility

Retain `worker.ffmpegConfig.videoCodec` as the selection key with supported values `av1_nvenc`, `libsvtav1`, and `libx265`. Set its registered default to `av1_nvenc`. Resolve omitted preset, CRF/CQ, and profile values per codec; reject incompatible explicit settings at worker startup. Keep `videoCRF`, `videoPreset`, and `videoProfile` for existing x265 configurations. Add `videoCQ` and `videoQMax` for NVENC. An omitted numeric override is represented by zero, since neither the chosen AV1 nor x265 profiles uses zero as a default quality target. Reject unsupported modes instead of emitting an invalid mixed command.

The NVENC profile is fixed to P010, p7, UHQ, VBR CQ34, zero target bitrate, full-resolution multipass, four middle-reference B-frames, disabled split encoding, and qmin0/qmax100. CQ, QP ceiling, and preset can be explicitly tuned. The SVT profile is 10-bit `yuv420p10le`, preset 3 and CRF23 by default, with preset and CRF overrides. This is a conservative CPU fallback from the tested preset-3 family; no single CRF was proven optimal for every source. The explicit x265 profile retains `yuv420p10le`, medium, CRF21, and main10 defaults.

The FFmpeg generator will return a structured argument slice and call `command.NewCommand`. This avoids ambiguous reparsing of paths and titles and makes exact argument tests possible. Existing `extraArgs` remain before input arguments, parsed with the project's current splitter; malformed or codec-specific overrides via `extraArgs` are outside the named profile contract and documented as advanced use.

### Concurrency

Add `worker.maxActiveJobs` (default 2) and `worker.ffmpegConfig.parallelJobs` (default 2). Admission applies both the active-job limit and optional time window. Run the configured number of consumers for the FFmpeg step queue; retain per-job action concurrency for PGS OCR separately. Each accepted job keeps its existing isolated workspace and progress tracker. Validate limits as positive at startup. This supports multiple sessions but makes no throughput claim for arbitrary hardware; operators tune limits for their GPU, CPU, disk, and OCR workload.

### Size and verification

Stat the complete MKV after FFmpeg exits, record the byte count in `jobContext.Target.Size`, and retain the existing verification rule that rejects outputs larger than sources. This prevents silent growth during the library migration. Preserve the original until the existing server completion policy runs; no source is deleted by the encoder itself.

## Risks / Trade-offs

- [Default NVENC requires a usable NVIDIA runtime] → Document GPU access and provide an explicit CPU profile; fail clearly when NVENC initialization fails.
- [A fixed CQ cannot guarantee p5 VMAF on all films] → Keep quality configurable and avoid claiming an automatic quality guarantee; use the existing manual pilot and visual review process.
- [CPU CRF23 may waste bytes on easy titles] → Make CRF configurable. Preset 3 was measured; further per-title tuning remains operator-driven.
- [Two simultaneous GPU encodes can contend for memory or I/O] → Bound both accepted jobs and encode consumers and expose both limits.
- [A larger output may now fail where the old zero target-size bug let it pass] → Report the measured sizes in the error; keep the original file available for retry or different settings.

## Migration Plan

Deploy PR #123's FFmpeg builder and PR #124's benchmark base first. GPU workers can use the new default once NVIDIA runtime access is configured. Set `worker.ffmpegConfig.videoCodec: libsvtav1` for CPU-only workers, or `libx265` to roll back. Existing x265 installations that explicitly set codec, preset, CRF, and profile retain that behavior. Configure `worker.maxActiveJobs` and `worker.ffmpegConfig.parallelJobs` to match each host's resources; set both to `1` to force serial processing.
