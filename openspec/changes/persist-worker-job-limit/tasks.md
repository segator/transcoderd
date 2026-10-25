# Tasks

## 1. Server persistence and API

- [x] 1.1 Add the additive PostgreSQL migration and repository methods for reading and upserting an optional maximum by worker name; verify non-negative values and absent settings in repository tests.
- [x] 1.2 Add authenticated worker settings read and update routes; verify authorization, validation, worker-name scoping, and error responses with handler tests.
- [x] 1.3 Document the worker settings routes in `openspec/PROTOCOL.md` and verify the route names, methods, auth, and request/response examples match the implementation.

## 2. Worker startup and runtime controls

- [x] 2.1 Add worker-client methods to read and persist the named worker's maximum; verify success, no override, server errors, and cancellation behavior in client tests.
- [x] 2.2 Resolve the server override before enabling new job requests, retaining restored work and retrying with admission closed when the server is unavailable; verify override, fallback, zero, and unavailable-server startup cases.
- [x] 2.3 Persist `set-max-jobs` before applying it; verify persistence failures do not mutate runtime state and successful changes resize encode concurrency.
- [x] 2.4 Document persistence, stable `worker.name` identity, restart behavior, and using zero to pause and a positive value to resume in worker deployment/control docs; verify the documented commands against CLI help and tests.

## 3. Integration and contract verification

- [x] 3.1 Extend the PostgreSQL integration test to persist and reload worker limits through a fresh repository, and verify worker startup applies positive and zero values before admission in unit tests.
- [x] 3.2 Run relevant Go unit and integration tests and `npx -y @fission-ai/openspec@latest validate --all --strict`; verify all tests and strict validation pass.
