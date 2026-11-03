# Design

## Shared dashboard

Move the existing HTML, CSS, and JavaScript into a Go-embeddable worker dashboard package. The Windows .NET project will continue to copy the same asset directory into its publish output. The UI transport adapter will use WebView messages on Windows and same-origin fetch requests when hosted by the worker.

## Linux/Docker host

Add optional `worker.dashboard.listenAddress` and `worker.dashboard.token` configuration. If configured, the worker starts an HTTP server serving embedded assets and authenticated `/api/status` and `/api/admission` endpoints. Status is read from the running worker's existing local control interface; pause/resume uses its persistent max-jobs control. Do not expose the central server queue or settings editing from this local-only host. The HTTP listener is absent by default. API authorization uses a constant-time bearer-token comparison.

The browser UI prompts for the dashboard token on first use and stores it in browser-local storage. The token is sent only to the same-origin API. The CSP allows same-origin API connections.

## Activity data and presentation

Extend local queued/running job and progress snapshots with the configured codec. Derive a display class from the codec: NVENC is GPU; SVT-AV1 and x265 are CPU. Preserve the original phase values and names. Add a visible, color-coded encoder badge independent of the existing phase badge. Activity filter chips are multi-select toggles for encoding, downloading, uploading and preparation; the all-activity chip clears other selections. The server queue remains unfiltered because it represents unassigned work rather than local activity.

## Version

Return the build-injected `version.Version` from both local status adapters and render it in the dashboard footer/sidebar. Do not infer version from a filename or installer.

## Security and failure behavior

The dashboard listener requires both an explicit address and non-empty token. Missing credentials prevent startup of that listener and produce a clear startup error. Static assets can load without credentials, but all status and control APIs require authorization. The browser sends no cross-origin requests. Existing Windows named-pipe/ACL security remains unchanged.

## Testing

Add unit coverage for API auth, status shape, static asset delivery, admission controls, codec classification, and UI filtering/rendering behavior. Build/test Linux and Windows targets; run the web UI tests using the project's existing JS test tooling if present, otherwise test pure filtering helpers through Node's built-in test runner.
