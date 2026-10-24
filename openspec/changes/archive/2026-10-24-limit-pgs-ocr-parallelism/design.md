# Design

## Context

See proposal.md - Why. The PGS OCR concurrency is currently registered as a CPU-derived pflag default in the worker entry point. `worker.pgsConfig.parallelJobs` already supports explicit overrides.

## Goals / Non-Goals

**Goals:**
- Bound default PGS OCR memory use to one active conversion per PGS phase.
- Keep the existing operator override available.

**Non-Goals:**
- Change total active worker jobs or FFmpeg encode concurrency.
- Introduce dynamic memory detection or runtime throttling.

## Decisions

- Change only the registered default to `1`; preserve the existing setting and execution semaphore. Operators with sufficient memory can still configure a larger value.
- Cover the registered command-line default with the existing worker flag-default test and update the user-facing configuration reference and subtitle spec.

## Risks / Trade-offs

- [PGS-heavy jobs may take longer because OCR actions run sequentially by default] → Keep the concurrency override configurable and document it.
- [Changing the default does not affect an already-running worker] → Recreate the worker during deployment so it loads the new default.
