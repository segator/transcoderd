# Design

## Context

The current Linux job serializes validation, Windows packaging and publication. A single native job downloads roughly 3.81 GB and runs both Windows suites sequentially. The `main` ruleset requires the `ci` check.

## Goals / Non-Goals

**Goals:** Reduce time on the critical path while preserving Dagger's warm cache, native validation, and immutable tagged-source completion.

**Non-Goals:** Change end-user installers, bundled languages or production update timing.

## Decisions

1. Resolve immutable source and version once. Start Linux checks, Windows distribution and Windows fixtures on independent runners. Run each native suite once its artifact arrives. Keep the `ci` aggregator as the required check over the same set of Linux validations and Windows build results.
2. Share a media tools node and use bounded parallel Dagger evaluations inside the Windows distribution target. Cache the ZIP of portable media tools independently of the linked worker. Compile the update harness binaries as shared Dagger files and transfer only those, ffprobe and the installers to the native update job.
3. Use English and Spanish traineddata for the PR update E2E media. Keep the same NSIS script, test binaries, signed fixture protocol, and failure cases. Releases and explicitly tagged completion use the full dependency payload. Tagged source can still use the legacy Mage targets and PowerShell signature gates.
4. Generate Linux release archives directly as Dagger files and transfer them from a Dagger container. Keep the existing GitHub release asset names and frontend platform variants.
5. Collect timing for native installer scenarios and compare normal CI with the existing baseline before tuning retry or recovery timers.

## Risks / Trade-offs

Parallel jobs add runner startup overhead and share an engine. Keep concurrent materialization bounded and compare job queue, packaging, transfer and native testing separately. Maintain the compatibility path for the manual debugger and older tags.
