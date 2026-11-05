# Design

## Context

See proposal.md for the operator's selected trade-off and docs/av1-svt-4-3-comparison.md for measurements. SVT default resolution currently supplies CRF25; both primary CPU profiles and the implicit CPU secondary profile use that resolver. Explicit overrides already work.

Job profile pinning currently copies an unresolved CRF0 into persisted contexts. The original profile-persistence implementation already used CRF25 (c98e80d), so these existing saved SVT profiles have an unambiguous legacy value. Applying the new resolver directly would change accepted work on restart, contrary to the immutable-profile contract.

## Goals / Non-Goals

**Goals:** New implicit SVT work uses preset 4 / CRF26; explicit overrides and saved work retain their intended quality; validation and references match the new default.

**Non-Goals:** Changing NVENC/x265, fractional CRF support, worker quality prepasses, retuning preset effort, or re-running the uncontended benchmark already completed for CRF26.

## Decisions

- Change only the SVT fallback to 26. The existing shared resolver covers CPU-primary and secondary configurations without duplicating quality logic.
- Resolve the selected SVT CRF before copying a new job's profile into its context. Keep the resolved value in subsequent serialized snapshots.
- For an already saved SVT profile with CRF0, copy the saved profile and materialize legacy CRF25 before resuming. Preserve explicit 25, 26 or any other valid override. Do not alter GPU or x265 saved-profile handling.
- Test real context persistence/reload for new implicit defaults, legacy zero and explicit qualities; also test command generation and secondary-profile defaults.
- Retain CRF25 as the controlled historical benchmark point. Update current user-facing defaults, Linux/native validation recipes and the report's final selection to CRF26.

## Risks / Trade-offs

- [Quality falls with smaller files] → Record measured mean/p5 losses and preserve the source/old/new comparisons; do not claim perceptual equivalence.
- [A changed fallback alters accepted work] → Materialize new SVT values and map legacy zero snapshots to the verified previous default, with disk round-trip tests.
- [Existing explicit configuration masks the new default] → Preserve overrides intentionally and document that explicit CRF25 remains 25.
