# Tasks

## 1. Worker admission and status

- [x] 1.1 Add a fixed two-job admission buffer while preserving zero-limit, schedule, and runtime limit behavior; verify with admission boundary tests.
- [x] 1.2 Report the accepted-job ceiling beside the encode target in local control status; verify text and JSON status tests.

## 2. Documentation and verification

- [x] 2.1 Update worker configuration and deployment documentation to explain the encode target and bounded pipeline buffer; verify references match implementation.
- [x] 2.2 Run worker unit tests, full Go tests, and strict OpenSpec validation; fix any failures and record results.
