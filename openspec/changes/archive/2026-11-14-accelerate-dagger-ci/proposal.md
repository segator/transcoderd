# Proposal

## Why

Six successful PRs took a median of 21 minutes even with warm media toolchains. Windows installer builds, artifact transfers and sequential native scenarios account for most of that time.

## What Changes

- Start Linux validation, Windows distribution packaging and Windows update fixtures independently from the resolved source.
- Run native distribution and update checks in separate Windows VMs. Use focused OCR data in ordinary update fixtures and complete fixtures for release validation.
- Publish Linux OCI archives and binaries from the Dagger Engine.
- Keep Dagger as the build engine and retain the required CI check and native release gates.

## Capabilities

### New Capabilities

### Modified Capabilities

- `distribution` specifies the full Windows installer payload used to validate signed releases.

## Impact

GitHub Actions dependencies, Mage and Dagger build graph, native Windows test inputs and release asset transfer.
