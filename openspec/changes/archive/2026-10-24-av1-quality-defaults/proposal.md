# Proposal

## Why

The current AV1 defaults prioritize quality conservatively and produce larger files than necessary for the balanced size, quality, and processing-time goal. Representative short-scene comparisons found lower quality targets reduce output size with a moderate VMAF change, so the defaults should reflect that measured trade-off.

## What Changes

- Change the default AV1 NVENC CQ from 34 to 38 while retaining p7/UHQ and all other NVENC parameters.
- Change the default SVT-AV1 CRF from 23 to 25 while retaining preset 4 and 10-bit output.
- Preserve explicit CLI and configuration overrides, and keep x265 defaults unchanged.
- Update encoder references and tests to describe and verify the defaults.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-encoding`: Change the default AV1 quality targets while retaining the rest of each encoder profile and explicit override behavior.

## Impact

Affected worker video-setting resolution, CLI help text, unit tests, OpenSpec configuration and media-encoding references, and the encoding profile documentation. No API, dependency, container, or runtime worker configuration changes are required.
