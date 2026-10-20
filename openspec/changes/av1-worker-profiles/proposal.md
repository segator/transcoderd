# Proposal

## Why

The worker's default x265 command cannot select an AV1 encoder correctly: it always adds HEVC-oriented profile, CRF, and pixel-format options. The completed RTX 5080 film pilot provides a user-approved NVENC AV1 recipe, while the benchmark lab identifies a usable software AV1 preset for workers without GPU access.

## What Changes

- **BREAKING:** Make AV1 NVENC the worker's default video encoder, using the exact video recipe from the approved full-film pilot: p7/UHQ, VBR CQ34, full-resolution multipass, four middle-reference B-frames, disabled split encoding, qmin0/qmax100, P010, and the existing 1920-pixel width cap.
- Add a selectable SVT-AV1 CPU profile with 10-bit output, preset 3, and a configurable CRF. Keep x265 available as an explicit rollback profile with its historical defaults.
- Validate encoder-specific settings at startup so AV1 does not accidentally receive x265 options, and describe the GPU/container requirements and CPU trade-off in configuration documentation.
- Make worker admission and simultaneous FFmpeg jobs configurable so multiple jobs can be processed in parallel within a bounded limit. Add optional weekly admission windows in an explicit timezone so operators can stop requesting new work during expensive electricity periods while accepted jobs finish.
- Populate the encoded file's actual size before verification so a larger result is rejected rather than uploaded as a storage-saving replacement.
- Exercise both AV1 profiles through generated-command tests, real encodes, output probing, and the container end-to-end path; update the OpenSpec contracts and operational docs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-encoding`: AV1 NVENC becomes the default; AV1 CPU and explicit x265 profiles have distinct output settings and validation.
- `job-dispatch`: Worker admission uses a configurable active-job limit, including when a time window is set, the encode phase can process multiple jobs up to a configured limit, and weekly windows can restrict new job requests without interrupting accepted work.
- `configuration-and-runtime`: The worker accepts and validates the new encoder and concurrency settings before starting job coordination.

## Impact

Worker CLI/YAML configuration, FFmpeg command generation, worker step scheduling, image-based E2E tests, and the configuration reference change. The server API and persisted job format do not change. GPU workers require access to the bundled NVENC-capable FFmpeg and NVIDIA runtime; CPU workers select `libsvtav1` explicitly. This PR depends on the FFmpeg image and benchmark work in PRs #123 and #124.
