# Design

## Context

The worker currently uses one active-job count for admission across download, subtitle processing, encoding, verification, and upload. FFmpeg concurrency is separately enforced by a dynamic encode pool. Each non-FFmpeg stage has one consumer. See proposal.md for motivation and the spec delta for the observable contract.

## Goals / Non-Goals

**Goals:**
- Let the worker admit a small bounded pipeline buffer beyond its encode target.
- Keep the configured maximum as the FFmpeg concurrency target and keep `0` as a hard stop for new requests.
- Make the effective admission ceiling visible in local status output.

**Non-Goals:**
- Change server assignment, job persistence, or upload semantics.
- Add a configurable buffer size or parallelize non-FFmpeg consumers.
- Interrupt or reorder accepted jobs.

## Decisions

- Use a fixed buffer of one accepted job beyond the configured maximum. This provides bounded overlap with preparation or verification/upload while encodes continue. A fixed bound avoids unbounded disk, memory, and transfer queues; a configurable buffer is deferred until operational evidence requires it.
- Keep `maxActiveJobs` as the configured limit and encode target. Admission checks compare active plus reserved jobs to `maxActiveJobs + 1`; zero remains a special case that disallows all new reservations.
- Extend local control status with the computed accepted-job ceiling and label the configured maximum as the encode target in CLI output. Existing JSON fields remain for compatibility.
- Preserve the current per-stage queue ordering and single-consumer behavior. The FFmpeg queue starts the next prepared job when a slot opens unless operator pause holds queued encodes. Maintenance drain allows accepted work to finish.

## Risks / Trade-offs

- [Extra accepted jobs increase temporary disk use and can overlap subtitle work with encoding] → Keep the buffer fixed at one and retain serial non-FFmpeg stage consumers.
- [A slow upload can still become the throughput bottleneck] → The buffer absorbs bounded overlap but does not increase upload concurrency or promise that encoding can never wait.
- [Large integer limits could overflow when adding the buffer] → Saturate the computed ceiling at the largest representable integer.

## Migration Plan

No server or database migration is needed. Existing worker settings retain their meaning as encode targets. On restart, the worker computes the admission ceiling from the effective persisted or startup maximum. Rollback consists of reverting the worker version; already accepted jobs continue under the existing persisted job state.

The `pause-queued-encodes` follow-up reduces the original two-job design and separates operator pause from maintenance draining.
