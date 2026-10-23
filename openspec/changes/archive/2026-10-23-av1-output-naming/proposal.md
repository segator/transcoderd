# Proposal

## Why

The server currently chooses a destination name before encoding and never checks which video codec the uploaded output actually contains. This can leave AV1 output mislabeled as `x265`, particularly when converting sources that are already HEVC.

## What Changes

- Probe the successfully uploaded output on the server and derive its final filename from the actual video stream codec.
- Map AV1 to the `av1` filename label and HEVC to the project's conventional `x265` label, regardless of the source filename or worker configuration.
- Rename the uploaded file and persist the final `target_path` before marking the job completed; never overwrite the source or an existing destination.
- Preserve the current provisional upload naming, audio-label normalization, and `deleteOnComplete` behavior after successful validation.
- Update the job-intake spec, naming documentation, and tests for AV1 and HEVC outputs from both H.264 and HEVC source names.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-intake`: the final destination filename and persisted path reflect the video codec detected in the completed output, including when the source is already HEVC.

## Impact

- Server-side output probing, target-name generation, filesystem rename, and job persistence ordering.
- `job-intake` OpenSpec requirement and scenarios.
- Server image FFprobe availability, naming documentation, and scheduler tests.
- No server configuration change is required. Jobs already queued can receive the correct final name when they complete; already completed jobs are not renamed retroactively.
