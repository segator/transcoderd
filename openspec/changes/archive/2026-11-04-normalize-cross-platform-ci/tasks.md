# Tasks

## 1. Workflow orchestration

- [x] 1.1 Move Windows artifact builds and lint into `ci`, preserve native and release/debug gates, add PR-only supersession cancellation, refresh deprecated action runtimes, remove confirmed unused workflow pieces and document the resulting graph; verify Actions syntax and release-source tests.

## 2. Dependency compilation

- [x] 2.1 Split independent Windows dependencies into Dagger branches, bound their aggregate compiler jobs to the configured budget and document override/default behavior; verify budget validation and real Windows cross-compilation with cache reuse.

## 3. Installer packaging

- [x] 3.1 Measure NSIS time and size on the real payload, apply the selected options and document their trade-offs; verify complete installer payload extraction and matching file digests.

## 4. Integration verification

- [x] 4.1 Run relevant Go/tooling tests, lint, real cross-builds and packaging, native bundled-tool execution, Linux E2E and strict OpenSpec validation; prepare the PR update with measured evidence and material limitations.

## Workflow follow-up

- Archive the completed tooling change after implementation and verification.
- Push the prepared PR update and inspect the resulting GitHub CI run; retain native installation/update validation as a release gate.
