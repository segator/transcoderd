# Proposal

## Why

The independent frontend image hard-codes an amd64 Go executable and cannot run on ARM64 hosts. The ordinary workflow also allocates separate runners for source resolution, JavaScript tests and overlapping Windows units, although these can share existing build/native jobs.

## What Changes

- Build and publish the frontend as one OCI image index with Linux amd64 and arm64 variants; validate both binaries, runtime health and index contents.
- Move immutable release-source validation and PWA/resource/activity tests into `ci`, executing their tooling through Dagger.
- Run the existing Windows unit coverage once in `windows-native`, including cross-compiled frontend tests, preserving legacy tagged-source compatibility.
- Move artifact-only Windows debugging into a dedicated manual workflow with read-only permissions; keep ordinary CI to `ci` and `windows-native`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `distribution`: frontend image tags and exported OCI archives support both Linux amd64 and arm64 while preserving image isolation, version identity and native Windows release gates.

## Impact

Mage/Dagger frontend builders, container publication/export, workflow orchestration, native test enumeration, release tooling and deployment/build documentation. Worker/server image architectures and frontend application APIs remain unchanged.
