# Tasks

## 1. Limit default PGS OCR concurrency

- [x] 1.1 Set the registered `worker.pgsConfig.parallelJobs` default to `1` and extend the worker flag-default regression test.
- [x] 1.2 Update the subtitle spec and configuration reference to document default `1` and the supported override.

## 2. Validate and release

- [x] 2.1 Run relevant worker tests and strict OpenSpec validation.
- [ ] 2.2 Create and merge a pull request, then publish the patch release.
- [ ] 2.3 Set the live local worker configuration to `parallelJobs: 1`, stop the current container, and recreate it with the released image.
