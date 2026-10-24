# Proposal

## Why

Worker progress output currently appends a new terminal line on every refresh, making long downloads and encodes difficult to follow. The Docker worker already runs with a TTY, so the console renderer should use that terminal correctly and redraw active progress in place.

## What Changes

- Require active worker progress trackers to update in place in an interactive terminal.
- Keep completed progress entries from remaining in the active display.
- Correct the progress tracker configuration so the renderer rewinds over active tracker lines before drawing the next frame.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `events-and-progress`: Specify that active local console progress is redrawn in place when an interactive terminal is available.

## Impact

- Affects the worker console renderer and its progress tracker lifecycle.
- Does not change server progress event frequency or payloads, job execution, or container startup configuration.
- The worker image must continue to be launched with a TTY for Docker log consumers to receive terminal control characters.
