# Tasks

## 1. Worker behavior

- [x] 1.1 Implement encode pause separate from maintenance drain, including zero startup, bounded concurrency and queued-job regression tests.
- [x] 1.2 Reduce the admission buffer to one and keep managed Windows profile limits authoritative before admission; cover mixed limits, stale server state and restored work.

## 2. Desktop and validation

- [x] 2.1 Update shared dashboard, tray and operational references to explain queued-encode pause, Resume, bounded prefetch and maintenance draining; test UI and backend paths.
- [x] 2.2 Run relevant Go/race/UI tests, Windows cross-build and strict OpenSpec validation; compare scenarios, synchronize and archive the change, then commit the reviewable fix.
