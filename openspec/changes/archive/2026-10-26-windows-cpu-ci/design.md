## Context

The existing Dagger cross-build exports the Windows bundle and unit executables.
The hosted Windows job already executes units and tools but cannot use the WSL
Dagger harness directly. windows-2025 provides PostgreSQL binaries via PGBIN.

## Goals / Non-Goals

Cover the complete native worker pipeline on every PR and gate Windows release
uploads. GPU execution is optional local validation; this adds no Windows server
product or server release artifact.

## Decisions

Cross-build a test-only Windows server in Dagger. A PowerShell harness creates a
private local PostgreSQL cluster with loopback-only trust, a unique database,
random ports and a temporary media source. Start the test server, reuse the real
worker E2E with libsvtav1, and stop only owned processes in finally blocks. Retain
server, database and worker logs in a predictable directory for artifact upload.
Use preinstalled PGBIN rather than installing Docker or a global database service.
The server test executable uses bundled media tools through its own bin directory.
Retain the existing release version/tag outputs and run Windows upload after E2E.

## Risks / Trade-offs

Hosted image tooling can change; fail clearly if PostgreSQL executables are absent.
The CI test uses a native test server; the separate WSL target keeps coverage of a
Linux server with a Windows worker. It is a fixture test, not a film benchmark.

## Discovered prerequisite

The native server exposed a source-download close bug: Windows rejects syncing a
read-only handle, preventing checksum publication and stalling the worker. Close
downloads without a write flush, retaining upload syncing, and cover read-only
checksum publication with a Windows regression. Record this in BASELINE.md.

## Validation

Execute the harness on real Windows with private PostgreSQL binaries and existing
bundle. Check the workflow with actionlint, run relevant Go regressions after
merging main, validate OpenSpec strictly, and review PR check results.
