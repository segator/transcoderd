# Verification

## Implementation and API boundaries
- Fleet HTML, JavaScript, styles, icons, manifest and service worker moved to frontend/web. API server production dependencies contain no frontend package; frontend production dependencies contain no server, worker or PostgreSQL packages.
- Real HTTP integration against the actual server verifies backend pages/assets return 404, session 204/401, token/cookie login, logout/expiry, credential separation, library listing, original 206 byte ranges and persisted independent CPU/GPU settings through the frontend.
- Frontend network tests verify same-origin mutation rejection, API allowlisting, forwarded-header/unrelated-cookie stripping, HTTPS cookie flags, redirect refusal, unavailable API handling, early stream flushing, conditional/range forwarding and upstream cancellation after disconnect.

## Automated checks
- Full Linux go test -race ./... passed.
- Native Windows frontend/web and frontend configuration test executables passed through Windows/WSL interop. CI windows-unit now includes frontend packages.
- Linux and Windows golangci-lint v2.11.4: 0 issues.
- Ten PWA/activity JavaScript tests and Windows desktop UI behavior checks passed.
- Mage module tests, build:all, build:frontend and docker:frontend passed. Native image export produced usable frontend version/branch tags; the untagged OCI-loader discrepancy is recorded in BASELINE.md.
- Strict OpenSpec validation passed before synchronization/archive.

## Isolated container and browser
- Ran the built frontend image as UID/GID 65532 with a read-only rootfs, dropped capabilities and no volumes. It had neither media folders nor FFmpeg/ffprobe/psql; only API network access.
- Login, remote folder browsing and original byte-range requests passed from that container. Chromium scanned a dedicated preview fixture, reviewed/selected it, queued exactly one real backend job and changed CPU/GPU limits through the container frontend. Test fixtures and synthetic state were reset afterward.
- Two-process Chromium checks passed for English login/errors, four views, mobile layout, preserved non-English filenames, source/output player choices, missing source playback, pause/resume and four rapid seeks with actual decoded preview output.
- Real Chromium PWA checks passed: no installability errors, explicit public-only cache, private-data exclusion, offline screen, reconnect/logout and two-version explicit update behavior.

## Delivery
The local preview now runs an independent frontend at http://localhost:18080/dashboard/ and an API-only backend at http://localhost:18081. Both are separate processes with synthetic monitoring data and test media, without production changes. Deployment settings, two-host/container examples, worker URL migration and API/session behavior are documented. New CI execution begins when the final change is pushed; no pending workflow is represented as completed here.
