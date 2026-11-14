## Context

The Windows update branch determines CI completion. Its time is driven by runner allocation before build and a deliberately failing health probe after installation.

## Goals / Non-Goals

**Goals:** Remove the release resolver from the PR critical path and shorten fixture execution while exercising native update and rollback.

**Non-Goals:** Change production health deadlines, install recovery semantics or the required status check.

## Decisions

1. Use the event merge SHA for PR builds and test the release resolver in Linux on Dagger. Resolve tags and release metadata before building in release flows.
2. Reuse the pinned Dagger CLI when Mage starts a session.
3. Give only the synthetic wrong-version health response a shorter budget in focused fixture builds. Preserve startup time and deadlines for full fixtures. Assert the observed recovery reason in the native scenario.
4. Keep focused fixtures for ordinary PRs and main pushes outside release publication. Keep full fixtures for release and manual validation.

## Risks / Trade-offs

Runner queue and hosted Windows runtime vary. Compare workflow and per-job timings, including at least one focused run and the complete fixture suite. A timing comparison with a concurrently dispatched run must identify the overlap.
