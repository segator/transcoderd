# Tasks

## 1. Frontend platforms

- [x] 1.1 Implement native cross-compilation, explicit runtime platforms, shared CA provisioning and multi-platform publish/export; verify real amd64/arm64 ELF binaries, runtime health and OCI index contents, and document deployment behavior.

## 2. CI consolidation

- [x] 2.1 Move source guards and web tests into `ci` with cached Dagger tooling that survives legacy tagged checkouts; verify guard tests, web tests and workflow syntax and document the two-job graph.

## 3. Windows coverage and diagnostics

- [x] 3.1 Merge native unit coverage into `windows-native`, include frontend test executables and legacy fallback, and move read-only artifact debugging into a manual workflow; verify Windows cross-compilation/native suites and workflow conditions and update operator references.

## 4. Integration verification

- [x] 4.1 Run relevant Go tests, both lints, frontend platform/index smoke checks and strict OpenSpec validation; prepare the PR with evidence and limitations.

## Workflow follow-up

- Sync the distribution delta and archive after implementation verification.
- Update the existing PR and inspect the ordinary two-job CI run.
