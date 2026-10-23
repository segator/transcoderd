# Tasks

## 1. Route mov_text through FFmpeg

- [x] 1.1 Exclude `mov_text` from external MKV extraction and map it from the source with the SRT encoder; verify unit tests cover direct mapping and mixed subtitle input indexes.
- [x] 1.2 Add a build-tagged end-to-end test that creates a short MP4 with `mov_text`, runs the generated FFmpeg command, and verifies subtitle codec, text, and full output decode.

## 2. Validate and prepare review

- [x] 2.1 Run focused subtitle and FFmpeg tests plus `openspec validate --all --strict`; record any environment-dependent skips.
- [x] 2.2 Review the diff for regressions to PGS and unsupported Matroska subtitle handling, then open a pull request.
