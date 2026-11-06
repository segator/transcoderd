# Design

## Context

WorkerRuntime already flows through heartbeat JSON, persisted JSON runtime, dashboard APIs and frontend resources. The version package contains the binary's injected release string, also used by local worker status.

## Decisions

Add optional runtime.version using version.Version. Reuse one escaped frontend version renderer across worker cards, overview rows and resource headings. Preserve missing values rather than deriving a release from controlVersion; offline labels explicitly identify last-reported information. Existing JSON storage carries the optional field without a schema migration. Do not compare with GitHub or initiate updates from the UI.

## Risks / Trade-offs

- [Older binary] → Its version remains unavailable until it runs a build supporting this telemetry.
- [Offline sample] → Keep the known value, label it last reported and retain basic offline presentation.
- [Untrusted heartbeat text] → Escape all displayed version values and tooltip content.
