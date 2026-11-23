## Why

An FFmpeg process can stop producing video progress indefinitely while retaining an encode slot and blocking queued jobs.

## What Changes

Detect stalled FFmpeg processes while excluding intentional pauses, restart the affected encode once, and fail it clearly if it stalls again. Leave other jobs and workers running.

## Capabilities

### New Capabilities

### Modified Capabilities

- `media-encoding`: Recover from an FFmpeg process that stops making video progress.

## Impact

Worker FFmpeg command execution and process lifecycle. Existing partial output is overwritten on retry.
