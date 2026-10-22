# Design: eMudhra trust remediation

## Context

The local TranscoderD server image was built on 2026-10-12. Its binary reports Go 1.25.0 and its runtime CA bundle predates eMudhra's revocation. Both supplied TLS test endpoints returned HTTP 200 from that image, including the Sectigo cross-signed chain.

The Go security fix is in Go 1.26.10 and later (and Go 1.27.3 and later). Fresh Debian Trixie images with the current `ca-certificates` package reject both endpoints. Keep the existing image distribution model and update these trust inputs at their existing build points.

## Decisions

- Pin the project's Go version to 1.26.10 in `go.mod`, `.mise.toml`, and Dagger build containers, so CI and local builds use the standard-library denylist.
- Keep installing `ca-certificates` during the runtime image build after `apt-get update`, ensuring a rebuilt image receives current trust data.
- Add endpoint scenarios to the distribution contract; no application-specific TLS bypass or custom certificate list is needed.

## Validation

- Confirm both endpoints fail TLS verification in a fresh Debian Trixie runtime environment.
- Build the server and worker binaries with Go 1.26.10 or later and verify the embedded toolchain version.
- Validate the OpenSpec change and run the relevant Go tests.
