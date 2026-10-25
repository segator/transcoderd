# Design

## Context

Successful finalization publishes the verified output, updates the job, and then removes destination-local and legacy staging files. Upload attempt files share the same staging directory, so cleanup must not remove a directory while it still contains another attempt.

## Goals / Non-Goals

**Goals:** Remove empty staging directories best-effort and keep a concurrent upload able to create its attempt file if cleanup races with directory creation.

**Non-Goals:** Change upload naming, output publication, job status handling, or cleanup of unrelated directories.

## Decisions

- Remove each staging file and then call `os.Remove` on its parent directory. `os.Remove` removes only an empty directory, so concurrent attempt files are preserved without a directory listing check.
- Treat a missing file/directory and a non-empty directory as expected cleanup outcomes. Log other cleanup errors and continue finalization because the final output and job record are already committed.
- If opening an upload attempt fails because the staging directory disappeared between `MkdirAll` and `OpenFile`, recreate the directory and retry once. This closes the race introduced by opportunistic empty-directory cleanup.
- Test destination-local cleanup, legacy cleanup, and preservation of a sibling attempt file.

## Risks / Trade-offs

- A filesystem can reject directory removal for reasons other than non-empty state → cleanup is best-effort and leaves the published output/job completion intact.
- Cleanup may race with another upload's directory creation → the writer retries after recreating the directory when `OpenFile` reports `ENOENT`.

## Migration Plan

No data migration is required. Deploying the server fix removes empty directories as jobs finalize; existing empty directories are not swept proactively.
