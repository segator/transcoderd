# Proposal

## Why

Release aliases such as `worker-v2` are currently rebuilt with `v2` embedded as the application version. The worker's semantic-version parser rejects that value during startup, so the moving tag can point to an unusable image.

## What Changes

- Publish release images under the full, minor, and major version tags from one image build that embeds the full semantic version.
- Keep branch tags and ordinary main-branch publishing behavior.
- Verify aliases resolve to identical image content and add regression coverage for tag construction.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `distribution`: Versioned and moving image tags must share the artifact built with the full semantic version.

## Impact

The release workflow and Mage image-publishing targets change. No application API or worker configuration changes. Existing release tags remain available; new releases will publish aliases without rebuilding with alias names as embedded versions.
