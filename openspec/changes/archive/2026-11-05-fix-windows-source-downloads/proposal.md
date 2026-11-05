# Proposal

## Why

The first main build after PR #206 failed while downloading FFmpeg with curl error 35 (TLS connection reset). Existing `--retry 5` does not retry that error, so a temporary transport failure aborts the whole pipeline.

## What Changes

- Retry failed Windows toolchain source GETs, including TLS resets and incomplete transfers, within explicit time and attempt limits.
- Keep checksum verification before extraction and remove temporary downloads on failure.
- Exercise the actual downloader against local TLS resets, partial responses and invalid checksums; document the retry boundaries.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. This is internal build reliability work (`skip_specs: true`). Distribution contents, validation gates and release authorization stay unchanged.

## Impact

Windows toolchain download helper, its Dagger inputs, focused Mage tests and CI documentation. No release is launched by this change.
