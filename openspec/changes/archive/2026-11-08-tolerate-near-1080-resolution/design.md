## Context

The worker currently uses 1920 both as a trigger and as the target width, including the possible rotated width.

## Goals / Non-Goals

Allow a modest 80-pixel margin without changing the 1920 target for larger sources or unknown dimensions. Do not change encoder quality or color settings.

## Decisions

Use 2000 as an inclusive threshold for both dimensions, conservatively accounting for FFmpeg autorotation. Known dimensions within the margin skip the scale filter and remain eligible for NVDEC. Larger or unknown dimensions retain the existing software path and 1920-target scale filter.

## Risks / Trade-offs

Near-1080 outputs can be slightly larger than before; hardware decoder support can vary and retains its initialization fallback.
