## 1. Shared dashboard contract and assets

- [x] 1.1 Move dashboard assets to a Go-embeddable shared location and update Windows packaging to consume the same files.
- [x] 1.2 Add installed version metadata to Windows and browser status, and render it in the UI.
- [x] 1.3 Add codec metadata to local job/progress snapshots and visually prominent CPU/GPU encoder badges.
- [x] 1.4 Add combinable activity filter chips for encoding, downloading, uploading, and preparation.

## 2. Linux/Docker dashboard host

- [x] 2.1 Add optional dashboard listen address/token configuration and validation.
- [x] 2.2 Implement authenticated status/admission HTTP API and embedded static asset server.
- [x] 2.3 Adapt the shared UI to use same-origin HTTP when hosted outside Windows WebView.
- [x] 2.4 Document Docker/Linux configuration and browser access.

## 3. Verification

- [x] 3.1 Add tests for dashboard API auth, status, control, assets, codec identity, and filtering.
- [x] 3.2 Run relevant Go tests, Linux build, Windows build, and strict OpenSpec validation.
