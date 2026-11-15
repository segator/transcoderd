## Why

Job submissions through the API reject MPEG transport stream files even though the media library can discover and queue them. Automated intake needs to accept `.ts` and `.m2ts` files directly.

## What Changes

- Accept `.ts` and `.m2ts` in API submissions for individual files and directories.
- Keep the existing minimum-size, duplicate-job, and Matroska output rules for these inputs.

## Capabilities

### New Capabilities

### Modified Capabilities

- `job-intake` will recognize MPEG transport stream extensions during file discovery.

## Impact

The server's extension validation, intake regression tests, and HTTP protocol reference change.
