# Design

## Context

`JobVerify` currently compares normalized container durations with an absolute tolerance. Matroska sources commonly omit per-stream duration metadata, and container duration can be set by subtitle timestamps beyond the video timeline. See the proposal and media-encoding delta for the behavior change.

## Goals / Non-Goals

**Goals:**
- Preserve the inexpensive current check for jobs whose container durations are already within tolerance.
- Use first-video-stream packet timestamps to resolve container-duration mismatches.
- Keep rejecting outputs whose actual video timeline is truncated or cannot be measured reliably.

**Non-Goals:**
- Change encoder arguments, the configured duration tolerance, or media stream mapping.
- Scan packets for every successfully verified job.

## Decisions

- Run a packet-level `ffprobe` scan only after the container-duration check exceeds the configured tolerance. This limits the extra I/O to mismatch cases while allowing subtitle/audio tails to be distinguished from video truncation.
- Measure the video timeline as the span from the earliest usable presentation timestamp to the latest presentation timestamp plus packet duration. Use DTS when PTS is unavailable for a packet; ignore malformed or non-finite timestamp fields.
- Treat a failed scan or a scan with no usable video timestamps as a verification error. Accepting an unmeasurable output would weaken the existing truncation guard.
- Compare the measured source and output video durations using the same configured tolerance.

## Risks / Trade-offs

- **Mismatch cases need an additional source and output packet scan** → This is limited to cases the current verifier would reject; normal successful jobs retain their current cost.
- **Damaged media may prevent a complete packet scan** → Fail verification with an explicit measurement error rather than silently accepting an uncertain duration.

## Migration Plan

No configuration or data migration is required. Deploy the worker update; existing jobs remain unchanged.
