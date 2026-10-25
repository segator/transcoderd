# Proposal

## Why

Completed jobs remove the staged upload file but leave its empty `.transcoderd-upload` directory beside the media, and legacy root staging directories can also remain. These empty directories accumulate across a large library and make successful finalization leave unnecessary filesystem clutter.

## What Changes

- Remove empty upload staging directories after finalization removes their staged files.
- Preserve a staging directory whenever another upload attempt or job still has files in it; cleanup errors remain non-fatal to a successfully published output.
- Cover both destination-local staging and legacy root-level staging.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-intake`: successful finalization also removes empty staging directories without affecting concurrent uploads or the final media file.

## Impact

- Affected code: `server/scheduler/scheduler.go` and scheduler tests.
- Affected behavior: filesystem cleanup after a completed upload; no API or database changes.
