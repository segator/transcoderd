## Why

PR 187 fails the Linux FFmpeg build because its upstream GitHub archive hash was
replaced with the hash of a different official release archive. The source
version is the same, but the bytes and therefore hashes are different.

## What Changes

- Restore the upstream GitHub archive/hash pair for Linux; retain the separately
  verified official archive/hash pair for Windows.
- Correct the source-provenance references and verify the CI gates.
- Measure Windows/WSL CPU and GPU encoding on the same movie excerpt, retaining
  exact inputs, arguments, timing samples, output payloads and decoded comparisons.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None. This restores the existing distribution contract and adds measured evidence.

## Impact

No encoder version, profile or release contract changes. The benchmark compares
existing profiles without assuming an operating-system performance winner.
