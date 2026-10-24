# Proposal

## Why

When Docker sends SIGTERM, the self-update supervisor terminates its child process. The child exits with a signal error, which the supervisor currently treats as a fatal panic; this prevents the process from completing its normal shutdown path.

## What Changes

- Treat child-process termination caused by a canceled service context as normal shutdown.
- Stop the updater loop and release its wait-group entry when shutdown is requested.
- Add regression coverage for cancellation while the supervised process is running.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `self-update`: A canceled supervisor SHALL stop its child process and exit without panicking or launching another child.

## Impact

The updater supervisor in `update/update.go` and its unit tests. No command-line flags, update policy, or runtime dependencies change.
