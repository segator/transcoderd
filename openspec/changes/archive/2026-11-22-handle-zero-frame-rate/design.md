# Design

## Context

FFprobe reports frame rates as integer ratios. Normalization already substitutes 24 fps when parsing returns an error, but division by zero panics before that fallback runs.

## Goals / Non-Goals

**Goals:** Preserve worker availability and the existing invalid-rate fallback.

**Non-Goals:** Change valid frame-rate calculations or infer a rate from other fields.

## Decisions

Reject a zero denominator before division and let normalization select its existing 24 fps fallback. Test both the parser and full normalization path.

## Risks / Trade-offs

The fallback estimates progress and does not change the encoded video's source timing.
