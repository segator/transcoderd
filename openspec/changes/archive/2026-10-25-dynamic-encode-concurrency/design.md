# Design

## Context

Job admission is changed through a local Unix-socket control command. The current implementation starts a fixed number of FFmpeg queue consumers once at worker startup, while all other pipeline phases use one consumer.

## Goals / Non-Goals

**Goals:** Resize concurrent FFmpeg job execution at runtime, preserve in-progress encodes, and make the current and target encode counts visible through local status.

**Non-Goals:** Change server assignment protocol, parallelize download/PGS/upload phases, or pause/cancel FFmpeg processes.

## Decisions

- Use one FFmpeg queue dispatcher with a runtime concurrency limiter. It holds the next queued FFmpeg job until a slot becomes available, then launches that job independently. Limit updates wake the dispatcher immediately.
- Positive `set-max-jobs N` updates both admission capacity and the FFmpeg target to N. A zero value only closes admission and leaves the last positive encoder target intact so accepted jobs continue through the pipeline.
- Lowering the target is non-preemptive. Current encodes finish; the dispatcher starts no further encodes while active encodes are at or above the new target.
- Keep one ordinary queue routine for each non-FFmpeg phase and add active/target encoder fields to the existing local status response.

## Risks / Trade-offs

- Lowering the target cannot reclaim encoder resources immediately because active FFmpeg jobs are not interrupted; the target is enforced as jobs complete.
- An explicit encode parallelism value remains the startup target, but an operator's later positive runtime max-jobs value intentionally replaces it.
