# Design

## Context

See proposal.md - Why. FFmpeg's normalized video currently omits pixel format and matrix metadata; subtitle direct conversion covers mov_text only. Extracted SRT files are accepted solely on conversion exit status. The scheduler searches the complete stderr, so harmless source listings and codec warnings can trigger retries.

## Goals / Non-Goals

**Goals:** Preserve selected streams, valid colors and immutable encode profiles while correcting the demonstrated input incompatibilities. Surface invalid subtitles before the expensive encode and preserve causal diagnostics.

**Non-Goals:** New memory admission policy, changing concurrency, unconditional CPU/GPU fallback, accepting larger outputs, global quality changes, or speculative native crash fixes. The operator explicitly deferred larger-output investigation.

## Decisions

- Extend normalized video with optional pixel format and color-space fields. Only an identity/gbr matrix on a known YUV pixel format enables `setparams=colorspace=unknown`. Missing legacy metadata and valid RGB/SDR/HDR take the existing path.
- Extend the established direct-SRT predicate to EIA-608. Reuse mov_text mapping and metadata handling; auxiliary subtitle indexes remain unchanged.
- Introduce shared converted-SRT validation/normalization. Empty successful text conversions use the existing empty-track representation. Canonicalize WebVTT whitespace separators and omit untimed annotation blocks while verifying that every timed cue survives conversion. Reject malformed timing blocks and tracks containing only unparseable nonempty text. Shield literal dialogue arrows from the SRT parser with ignored span tags, preserving cue text, styling and timing. Use atomic replacement for rewritten subtitles and keep track identity in errors. Capture external conversion stderr.
- Replace broad warning-based retry rules with explicit transient causes and deterministic-error precedence. Keep five-minute maintenance and the existing ten-failure guard. Memory errors qualify for bounded retry; unclassified native crashes remain diagnostic failures.
- Unit coverage covers mixed logs, stream indexes, metadata, legacy contexts and malformed/empty subtitles. Real-tool tests verify actual output and decoding, with focused original-source reproductions where available.

## Risks / Trade-offs

- [Clearing valid HDR metadata] → Conditional predicate and unchanged transfer/primaries, with regressions for SDR, HDR, RGB and legacy metadata.
- [Losing subtitle text during recovery] → No cue-dropping recovery; text/timing assertions and fail on unrecoverable content.
- [Over-retrying native or input failures] → Explicit causes, mixed-log regressions and unchanged attempt limit.
- [Older media tools differ] → Validate with installed Windows bundle and Linux FFmpeg; document actual tool versions and test boundaries.

## Migration Plan

No schema or configuration migration. Deploy through ordinary release handling after CI and review. Operator-authorized RAM/VRAM requeues use the authenticated existing API and skip completed/already-active jobs. Rollback uses the previous release and does not alter saved profiles.
