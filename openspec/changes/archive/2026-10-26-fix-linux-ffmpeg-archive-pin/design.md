## Context

The upstream v9.0.10 recipe downloads GitHub FFmpeg n9.0.2 with SHA-256
6e374ed621e48faa40639307dff48ba6fe574a509977956d2cce9669b7cc27e9.
The Windows recipe downloads the official signed tarball with hash 84960df9... .
The Linux sed override was erroneous and caused the observed CI mismatch.

## Decisions

Remove the Linux override and retain the upstream verified pair. Record the
mistaken provenance assumption in BASELINE.md and correct validation wording.
Require native Windows CI and Linux checks before merging the PR.

For the requested local comparison, use identical hashed movie excerpt bytes,
production CPU SVT preset 4/CRF25 and GPU p7/UHQ/CQ38 settings, local filesystems,
sequential alternating timed runs and no production encodes overlapping. Report
build/compiler differences, sizes, frame counts and decoded/frame comparisons.
Keep short-clip measurements separate from completed-film benchmarks and do not
attribute every timing difference solely to WSL virtualization.
