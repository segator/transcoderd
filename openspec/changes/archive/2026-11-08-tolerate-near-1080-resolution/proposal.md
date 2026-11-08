## Why

Small differences above 1920 pixels should not trigger a resize and prevent eligible GPU decoding.

## What Changes

- Allow source dimensions up to 2000 pixels without a resize.
- Continue reducing larger source widths to 1920 pixels.
- Apply the same eligibility threshold to NVDEC.

## Capabilities

### Modified Capabilities
- `media-encoding`: Update resolution and GPU decoding thresholds.

## Impact

Worker FFmpeg command generation, output dimensions, and the encoding profile reference.
