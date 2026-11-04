# Verification

- Reviewed application-owned text across the server dashboard, shared worker UI, Windows desktop and Go runtime messages. Translated the server surfaces; user-provided paths, filenames, worker names and Unraid path-matching preset values remain unchanged.
- `go test -race ./server/web ./server/library`: passed, including the updated offline reconnection assertion.
- JavaScript syntax checks, ten PWA/activity lifecycle tests and `scripts/windows/test-web-ui.cjs`: passed.
- Chromium checked English login/token errors, all four dashboard views, worker limit controls, empty job results, library root labels/API errors, embedded player choices, English manifest/offline reconnect and desktop/mobile layout. A Spanish source filename remained unchanged.
- HTML structure, IDs, input names, routes and functional attributes match the previous version. Source inventory finds no remaining Spanish application copy in the dashboard/worker/desktop surfaces.
- Strict OpenSpec validation passed. Preview rebuilt and running at http://localhost:18080/dashboard/ with the existing static dashboard credential.

These checks validate this copy-only change. The previously completed full CI run 41793932580 applies to parent commit e41ce9e; pushing this change starts a fresh pipeline and does not imply its completion.
