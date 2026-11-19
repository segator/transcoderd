## Why

Workers returning after an assignment timeout can restore expired work and retry rejected events indefinitely. DNS failures during initial registration also cause supervised workers to restart repeatedly.

## What Changes

- Retry initial presence with cancellable, bounded requests.
- Check saved assignments before restoring any work and discard expired local workspaces.
- Use the server's latest event sequence when resuming a current assignment.

## Capabilities

### Modified Capabilities

- `configuration-and-runtime`: Retry initial presence until connectivity returns.
- `maintenance-and-recovery`: Validate local restart recovery against server assignments.

## Impact

Worker startup, persisted job workspaces and the existing authenticated job lookup.
