# Proposal

## Why
PR #202 fails the native Windows pipeline because the pinned CPU/GPU profile execution test installs a Unix shell script as its fake FFmpeg. Windows cannot execute that fixture, so neither test case writes its captured arguments.

## What Changes
Replace the shell fixture with a copy of the native Go test executable that captures its actual command arguments as JSON and exits with a distinct code. Preserve both execution-level CPU/GPU isolation checks and validate the exact fixture exit code. Run the same tests natively on Linux and Windows, include worker/step in the early Windows unit job, then recheck the PR pipeline.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None. This is a test-fixture portability fix; runtime behavior and configuration do not change. The change sets skip_specs.

## Impact
worker/step tests and verification evidence only. Existing required checks remain enabled.
