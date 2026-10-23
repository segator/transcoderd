# Proposal

## Why

Jobs with MP4 `mov_text` subtitles currently fail because the worker sends the MP4 to `mkvextract`, which only accepts Matroska input. FFmpeg can read the subtitle stream from the MP4 and convert it directly to SRT while creating the output MKV.

## What Changes

- Convert `mov_text` subtitle streams directly from the source using FFmpeg's SRT subtitle encoder.
- Keep PGS OCR and extraction of other unsupported text codecs on their existing paths.
- Add an integration test that creates a small MP4 containing `mov_text`, runs the generated encode command, and inspects the resulting MKV subtitle stream and text.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Exclude `mov_text` from the MKVExtract phase and convert it directly to SRT during encoding.

## Impact

- Affects subtitle codec classification, FFmpeg argument generation, worker step scheduling, and subtitle tests.
- No configuration, API, database, or image dependency changes.
