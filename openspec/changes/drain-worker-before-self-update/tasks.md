# Tasks

## 1. Drain worker jobs before an update restart

- [x] 1.1 Stop job admission after update detection and wait for active jobs to finish before requesting supervisor restart; add tests for active-job draining and cancellation.
- [x] 1.2 Return the child update exit code to the supervisor and force an immediate release recheck; add regression coverage for invalidating the cached check.

## 2. Validate the change

- [x] 2.1 Run the relevant worker and updater tests and strict OpenSpec validation; resolve any failures.
