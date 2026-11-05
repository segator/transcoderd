# Tasks

## 1. CPU default and command generation

- [x] 1.1 Change the omitted SVT CRF to 26, update CLI/help and current profile references, and verify primary/secondary defaults, explicit CRF25 overrides and generated FFmpeg arguments with Go tests.

## 2. Stable saved quality

- [x] 2.1 Materialize the new SVT CRF in new job snapshots and restore legacy saved CRF0 as 25; verify disk persistence/reload across new defaults and changed startup settings, and record the resolved discrepancy in BASELINE.md.

## 3. Distribution and integration

- [x] 3.1 Update Linux/native Windows CPU gates to preset 4 / CRF26 and verify linked 4.3.0, 10-bit decode and the complete native CPU worker E2E; preserve the historical benchmark and document the operator-selected CRF26 trade-off.
- [x] 3.2 Run relevant Go/Python checks and strict OpenSpec validation, synchronize the media-encoding/distribution deltas, and confirm explicit overrides and unrelated codec defaults remain intact.

## Workflow follow-up

- Archive the completed profile change after validating its delivered scenarios.
- Publish the combined SVT-AV1 upgrade and CRF26 profile branch and open a reviewable PR against current main, preserving release and dependency updates already on main.
