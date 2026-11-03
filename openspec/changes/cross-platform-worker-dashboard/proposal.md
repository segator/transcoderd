# Proposal

## Why

The current worker dashboard is tied to the Windows tray WebView, so Linux and Docker workers cannot expose the same useful live view. The dashboard also hides encoder identity in secondary text and provides no way to focus activity by phase.

## What Changes

- Serve the shared English dashboard from a Linux/Docker worker over an optional authenticated HTTP endpoint.
- Show the running application version and a prominent encoder badge on every local job.
- Add activity filters that can be combined to show encoding, downloads, uploads, and preparation work.
- Keep the Windows tray dashboard on the same shared UI and preserve its settings workflow.

## Capabilities

### New Capabilities
- `worker-dashboard`: Browser dashboard hosting and activity presentation for a running worker.

### Modified Capabilities
- `windows-desktop-worker`: More visible encoder identity and activity filtering in the existing dashboard.
- `events-and-progress`: Include the configured encoder identity in the worker's local job snapshot.

## Impact

Affected areas include worker configuration and startup, the authenticated local control/status surface, shared dashboard assets, Windows desktop packaging, worker progress snapshots, documentation, and tests. The dashboard listener is disabled unless explicitly configured and requires a bearer token.
