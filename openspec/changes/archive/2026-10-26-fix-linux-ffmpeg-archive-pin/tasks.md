## 1. CI repair

- [x] 1.1 Restore the Linux source pin and correct provenance references; verify the archive hashes and relevant build/spec checks.
- [x] 1.2 Push the repair and ensure Linux and native Windows CI checks pass.

## 2. Local comparison and completion

- [x] 2.1 Run equivalent CPU/GPU movie-excerpt encodes on Windows and WSL without overlapping production jobs; retain timing and output evidence.
- [x] 2.2 Compare decoded output, frame counts, payload sizes and performance, document limits, and archive this fix.

## Workflow follow-up

Merge PR 187 only after required checks and validation succeed.
