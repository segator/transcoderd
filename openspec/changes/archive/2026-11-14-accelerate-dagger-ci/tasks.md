## 1. Dagger build graph

- [x] 1.1 Share Windows media tool and native test build nodes. Reuse the portable media ZIP across worker rebuilds and bound concurrent packaging.
- [x] 1.2 Build focused PR update fixtures and full release fixtures. Export the exact inputs each native job needs and time update phases.
- [x] 1.3 Upload Linux release assets directly from their Dagger graph.

## 2. GitHub orchestration

- [x] 2.1 Split the build and native test dependencies. Preserve the required check, debugger, tagged source, release signing, and publication gates.
- [x] 2.2 Update the pipeline reference with cache, workflow and measurement details.

## 3. Validation

- [x] 3.1 Run relevant Go tests, strict OpenSpec validation, and workflow checks.
- [x] 3.2 Verify native GitHub CI and record timing and artifact sizes.
