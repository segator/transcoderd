# Proposal

## Why

The current duration check compares container durations, which can be extended by subtitles or other streams beyond the actual video. This rejects valid encodes even when the source and output video timelines match, while duration verification must continue to catch genuinely truncated video.

## What Changes

- Keep the current fast container-duration check for normal jobs.
- When container durations exceed the configured tolerance, compare the first video stream packet timelines before failing the job.
- Report a duration failure only when the measured video durations exceed the configured tolerance; fail safely when the video timeline cannot be measured.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-encoding`: duration verification is based on the video timeline when container duration comparison indicates a mismatch.

## Impact

- Affected worker code: FFmpeg probing and `JobVerify`.
- Affected tests: duration verification and packet-duration parsing.
- No CLI, configuration, or wire protocol changes; `worker.verifyDeltaTime` remains the tolerance.
