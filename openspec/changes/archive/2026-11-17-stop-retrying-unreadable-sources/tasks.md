# Tasks

## 1. Protect failed source reads

- [x] 1.1 Guard subsequent reads for the same assignment after a source read error; test retry, new assignments, expiry and concurrent access.
- [x] 1.2 Publish checksums only for complete transfers; test source failures and client disconnects.
- [x] 1.3 Validate worker compatibility, relevant Go tests and strict OpenSpec validation.

## Workflow follow-up

- Sync and archive the validated change.
