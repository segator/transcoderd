# Tasks

## 1. Encoder defaults and coverage

- [x] 1.1 Change omitted AV1 NVENC CQ to 38 and omitted SVT-AV1 CRF to 25; update resolver tests and verify `go test ./worker/config/...` passes while override and x265 cases remain unchanged.
- [x] 1.2 Update CLI help and generated FFmpeg command expectations; verify `go test ./worker/...` passes.

## 2. References and validation

- [x] 2.1 Update the media-encoding spec, configuration reference, and encoding profile documentation; verify all documented defaults agree.
- [x] 2.2 Run `npx -y @fission-ai/openspec@latest validate --all --strict` and the relevant Go tests; resolve any failures.
