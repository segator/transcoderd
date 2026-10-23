# Tasks

## 1. Tag planning

- [x] 1.1 Extend Mage image-tag generation to include release aliases while retaining full-version and branch tags; add focused tests for trimming, empty entries, and both image roles. Verified with `mise exec -- go test ./...` in `magefiles/`.
- [x] 1.2 Update the release workflow to invoke app publication once with the full, minor, and major versions, and verify the generated workflow YAML and Mage target behavior.

## 2. Contract validation

- [x] 2.1 Run Mage tests and strict OpenSpec validation; verify all release tags are produced from one canonical full-version container definition. Verified with Mage module tests, root `go test ./...`, workflow YAML assertions, and `openspec validate --all --strict`.
