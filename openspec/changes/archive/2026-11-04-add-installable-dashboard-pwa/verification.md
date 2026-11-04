# Verification

Verified on 2026-11-04 with Go 1.26.10, Node 24 and real Chromium. The local review server uses isolated synthetic workers/statistics and test clips; no production data or queue was changed.

- `go test -race ./...`: passed. Focused server/web race tests passed after the final asset changes.
- HTTP tests validate public manifest/PNG dimensions/MIME, worker scope/CSP/no-store, deterministic worker version, metadata links, and continued login/logout authorization.
- `node --test server/web/dashboardPWA_test.cjs worker/dashboard/activity_test.js`: 10 passed. Covers public-only credential-free precaching, owned-cache cleanup, bypass for APIs/media/credentials/mutations, offline fallback, online auth failures, non-creating runtime lookups, and user-controlled update reload.
- golangci-lint v2.11.4: 0 issues. Linux server and Windows amd64 server builds passed. Diff whitespace and JavaScript syntax checks passed.
- Real Chromium with a normal persistent test profile reports no manifest or installability errors. (Incognito profiles intentionally cannot install applications.)
- CacheStorage inspection after authenticated statistics/library/media requests contains only the 13 public allowlisted assets; no authenticated pages or API/video responses are retained.
- Offline reload presents the generic reconnect page without operational UI/data; API requests remain failed. Reconnect checks the live server. Logout works at 390 px and subsequent online navigation/API access requires authentication.
- An isolated local proxy supplies a second worker version to exercise installation/activation: obsolete dashboard cache disappears, an unrelated app cache remains, update notice appears, unsaved page state survives until the operator explicitly reloads.
- Public icons include regular 192/512 PNGs, a separate safe-padded 512 maskable icon and a 180 Apple icon derived from the existing vector brand.
- CI runs the lifecycle/privacy and shared activity tests in the pwa-tests job. Strict OpenSpec validation passes after syncing the operations-dashboard capability.

Native operating-system installation was not automated; browser installability and actual service-worker/offline/update behavior were verified. Production installation requires HTTPS; localhost/loopback works for this development preview. Private pages and live controls intentionally do not operate offline. The preview remains running at http://localhost:18080/dashboard/ for user review, with default token encoding123. PWA changes are included in the existing PR #202.
