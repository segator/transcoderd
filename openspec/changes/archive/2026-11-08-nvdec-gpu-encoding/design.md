# Design

## Context

Windows FFmpeg lacks scale_cuda. Hardware frames can feed AV1 NVENC directly, with internal 8-to-10-bit conversion. Existing saved contexts may lack codec and dimensions.

## Goals / Non-Goals

**Goals:** Use hardware decoding for proven inputs, preserve 1920-width policy and make decoder startup failures recoverable.

**Non-Goals:** Enable hardware decoding for CPU encoders or change 4K output resolution.

## Decisions

Persist optional source codec and dimensions in the normalized probe. When metadata is absent, retain the old software pipeline. Restrict hardware decoding to H.264 and HEVC 4:2:0 with both dimensions at most 1920, valid color metadata and no custom input arguments. Decode hardware frames directly into NVENC; use its 10-bit conversion for 8-bit surfaces. On a recognizable hardware initialization error before progress, retry once with software decoding. Preserve other errors and cancellation. Sources with both dimensions at most 1920 omit the scale filter. Larger or unknown dimensions retain the old filter, since FFmpeg may autorotate a tall source into a wider output.

## Risks / Trade-offs

- Hardware capabilities vary by codec and driver → conservative eligibility and initialization fallback.
- Windows CUDA scale is absent → wide inputs retain the existing CPU scaling path.
- Resuming old jobs lacks metadata → preserve software behavior rather than guessing.
