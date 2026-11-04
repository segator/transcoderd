# Proposal

## Why

The operator selected preset 4 / CRF26 after reviewing the measured SVT-AV1 4.3.0 results. Three film excerpts used 14.3–16.0% fewer video bytes than the prior 4.2.0 / CRF25 profile, with mean VMAF reductions of 0.286–0.363 point; this is an explicit size/quality trade-off.

## What Changes

- Change the omitted SVT CPU CRF from 25 to 26 while retaining preset 4, 10-bit output and the existing scaling rule.
- Keep explicit quality overrides and GPU/x265 defaults intact.
- Store the resolved SVT CRF when selecting a new job profile; interpret an existing saved SVT profile with CRF0 as legacy CRF25 so the default change does not alter accepted work on resume.
- Validate the default CPU recipe in Linux toolchain and native Windows checks, update operational references, and include the benchmark trade-off in the PR.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `media-encoding`: CPU CRF26 default and stable per-job SVT quality across the default transition.
- `distribution`: Run the bundled CPU encoder version/10-bit checks with preset 4 / CRF26.

## Impact

Worker profile resolution, profile pinning/restoration, configuration/FFmpeg-generation tests, media-tool gates, and encoding/configuration references change. The existing saved-profile discrepancy is recorded in BASELINE.md and resolved in this change. Version-comparison CRF25 measurements remain historical evidence; no production worker is reconfigured by the PR.
