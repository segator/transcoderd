# Proposal

## Why

A 30-day production audit found deterministic failures repeatedly scheduled on healthy workers: EIA-608 copied into Matroska, an identity RGB matrix on YUV video, and invalid converted subtitles. Generic warning text in full FFmpeg logs currently masks the actual failure during retry classification.

## What Changes

- Convert selected EIA-608 captions directly to SRT, preserving selected subtitle metadata.
- Clear impossible identity RGB matrix metadata only for YUV sources; preserve valid SDR/HDR metadata and existing encoder profiles.
- Validate and normalize extracted/OCR SRT before encoding, handle empty tracks explicitly, and retain useful conversion diagnostics.
- Retry explicit transient resource/network failures within the existing attempt limit; stop treating generic stream/encoder warnings as recoverable causes.
- Requeue the remaining audited RAM/VRAM failures operationally, without new memory admission policy.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Direct caption conversion and valid converted subtitle inputs.
- `media-encoding`: Conditional correction of invalid source color matrix metadata.
- `maintenance-and-recovery`: Causal retry classification with deterministic failure precedence.

## Impact

Worker probe normalization, FFmpeg generation, subtitle conversion and scheduler classification, with unit and real-tool regression coverage. No API, schema, encoder quality default or concurrency changes. Size and duration guards remain enforced. Unexplained native crashes require evidence rather than speculative codec changes.
