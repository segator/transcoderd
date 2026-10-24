# Design

## Context

Encoder-specific defaults are resolved in worker configuration when numeric values are omitted or set to zero. Explicit non-zero CLI/YAML values already flow through the same resolver. The AV1 profile includes additional fixed NVENC and SVT settings that this change must preserve.

## Goals / Non-Goals

**Goals:**
- Change only the default AV1 quality targets and make the resolved values visible in tests, help, and operational references.
- Keep profile selection, preset, pixel format, rate-control mode, overrides, and x265 behavior stable.

**Non-Goals:**
- Add per-title analysis, VMAF gating, adaptive retries, or new encoder options.
- Change active worker configuration, deployment, or release behavior.

## Decisions

- Keep zero as the unset sentinel and change the fallback constants in the encoder-specific resolver. This preserves existing configuration compatibility and ensures explicit values still win.
- Update tests for both omitted settings and explicit overrides. The x265 test remains a guard that its CRF stays 21.
- Document the change as a measured size/quality trade-off, not a guarantee of any VMAF floor. Short-scene results are content dependent and do not establish full-library performance.

## Risks / Trade-offs

- [Risk] Higher CQ/CRF can lower quality, especially in grain, high motion, and other difficult scenes → Retain explicit overrides and document that the defaults do not guarantee a VMAF threshold.
- [Risk] Existing operators may rely on the previous implicit quality values → Explicit values preserve the former behavior when needed.

## Migration Plan

No configuration migration is required. Workers selecting AV1 and omitting CQ/CRF receive the new defaults after updating; explicitly configured values are unchanged. x265 remains unchanged.
