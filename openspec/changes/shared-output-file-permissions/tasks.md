# Tasks

## 1. Shared output permissions

- [x] 1.1 Apply mode `0664` to published output files before completion is persisted, and add a regression test that verifies the resulting mode.
- [x] 1.2 Validate the OpenSpec change with `npx -y @fission-ai/openspec@latest validate --all --strict` and run `go test ./server/scheduler/...`.
