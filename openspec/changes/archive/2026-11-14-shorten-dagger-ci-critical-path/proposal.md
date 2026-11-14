## Why

The last two warm PR runs spent 58 and 171 seconds waiting for the Windows fixture builder. The update E2E also waits through the production deadline for an intentionally wrong-version response.

## What Changes

- Start PR builds from GitHub's immutable merge SHA and run release-source guard tests in the Linux Dagger job.
- Reuse the Dagger CLI installed on the runner.
- Shorten only the intentionally failed health probe in focused update fixtures and assert the specific recovery cause.
- Use focused update fixtures for main pushes without a release. Releases, tag completion and manual validation exercise the full payload.

## Capabilities

### New Capabilities

### Modified Capabilities

- `distribution` specifies the focused and full Windows update validation cases.

## Impact

GitHub scheduling, Dagger bootstrap, Windows fixture timing and distribution validation.
