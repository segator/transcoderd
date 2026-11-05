# Tasks

## 1. Download reliability

- [x] 1.1 Add the bounded, checksum-verified downloader to the Dagger Windows stages and document its limits; verify actual curl tests recover TLS resets and partial transfers, reject bad checksums and stop after exhausted retries.

## 2. Integration validation

- [x] 2.1 Run Mage tests/vet, shell syntax, strict OpenSpec validation and an actual Windows cross-build through Dagger; verify the repaired downloader feeds a successful portable build.

## Workflow follow-up

- Archive the completed tooling change without spec deltas.
- Submit the repair as a PR; do not merge the release PR or dispatch publication.
- Verify the PR run passes the changed build and existing native Windows gates.
