# Tasks

## 1. Runtime admission policy

- [x] 1.1 Replace the fixed worker concurrency limit with a thread-safe runtime admission state that defaults to two; add tests for zero disabling admission, changing limits above and below active jobs, schedule enforcement, and reservation races.
- [x] 1.2 Reserve admission capacity before each server request and release it on empty/error responses; verify the coordinator cannot oversubscribe when the limit changes during a request.
- [x] 1.3 Allow zero as the configured admission maximum, remove the separate paused setting, and ensure persisted jobs can continue after worker restart while admission is disabled.

## 2. Local worker controls

- [x] 2.1 Add a private Unix-socket control service and `control status|set-max-jobs` client commands; test command validation, responses, and socket access.
- [x] 2.2 Wire the control service into worker startup/shutdown and add an optional non-negative startup maximum defaulting to two; verify CLI defaults and graceful socket cleanup.

## 3. Documentation and integration verification

- [x] 3.1 Document runtime commands, container invocations, persistence across restart, zero-limit behavior, and the interaction with schedules; verify examples match the CLI.
- [x] 3.2 Run relevant worker tests, race tests, and `openspec validate --all --strict`; resolve any failures.
