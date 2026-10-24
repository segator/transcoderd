# Proposal

## Why

The worker currently derives the default PGS OCR concurrency from the detected CPU count, which can start several memory-intensive PgsToSrt processes at once. On the 16 GiB WSL worker this caused high memory pressure and an OOM indication, so the default should favor stable memory use while remaining configurable.

## What Changes

- Set the registered default for `worker.pgsConfig.parallelJobs` to `1`.
- Preserve the existing configuration override so operators can choose a higher OCR concurrency when their worker has enough memory.
- Update the worker configuration reference and the subtitle behavior contract.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Change the registered default for concurrent PGS OCR actions to one.

## Impact

- Affects the worker CLI default and PGS OCR concurrency when no explicit value is configured.
- Affects the subtitle specification and worker configuration documentation.
- Explicit `worker.pgsConfig.parallelJobs` values remain supported.
