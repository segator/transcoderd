# Proposal

## Why

A manually published stable release can lack validated Windows assets. Release Please does not emit release_created again, leaving no supported way to finish that release.

## What Changes

- Add an explicit existing-release completion input to the ordinary validation workflow.
- Resolve and validate the immutable release source before building, retain all native Windows gates, and publish signed metadata last.
- Refuse replacement of already signed Windows releases and document recovery.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `distribution`: guarded completion of existing stable releases without changing their source or already authenticated Windows payloads.

## Impact

GitHub Actions, release source validation tooling and Windows release operations documentation. No runtime behavior changes.
