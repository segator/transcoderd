# AV1-ready FFmpeg distribution

## Why

The worker currently consumes a prebuilt FFmpeg image and defaults to HEVC. AV1 comparisons need a recent FFmpeg binary with both software and NVIDIA NVENC AV1 encoders available before changing job settings or the encoding pipeline.

## What Changes

- Build FFmpeg 9.0.2 from the pinned `v9.0.10` upstream build script with CUDA development tools, SVT-AV1, libaom, and NVENC.
- Publish and consume a versioned FFmpeg builder image so application images cannot silently reuse the previous FFmpeg binary.
- Check the required AV1 encoders and the existing HEVC and AAC encoders during image construction.
- Keep the worker's HEVC defaults and its encoding command unchanged.

## Impact

- Affected capability: `distribution`.
- Affected files: `Dockerfile`, `Makefile`, `.github/workflows/main.yml`, and OpenSpec distribution documentation.
- The new builder tag must exist in GHCR before ordinary CI builds can avoid compiling FFmpeg; CI builds it if missing.
