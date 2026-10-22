# Proposal

## Why

TranscoderD's local server and worker images currently accept TLS connections using compromised eMudhra intermediate certificates. The checked images contain binaries built with Go 1.25.0 and an older CA bundle, so they can connect to both official test endpoints.

## What Changes

- Upgrade the project Go toolchain to a release containing Go's eMudhra intermediate denylist.
- Ensure freshly built runtime images install the current Debian CA bundle.
- Require server and worker image TLS clients to reject the compromised test chains.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `distribution`: Require supported Go certificate verification and a current runtime CA bundle to reject the compromised eMudhra intermediates.

## Impact

The Go module and toolchain pin, Docker runtime image construction, distribution behavior, and server/worker images.
