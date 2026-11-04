# Tasks

## 1. Runtime control and telemetry

- [x] 1.1 Synchronize desired capacity before each admission cycle, including while paused; cover remote changes, failures, and local-control serialization in worker tests and document rollout.
- [x] 1.2 Add optional heartbeat telemetry and an additive migration; verify modern and legacy worker presence in unit and PostgreSQL integration tests.

## 2. Dashboard backend and authentication

- [x] 2.1 Implement SQL fleet aggregates, bounded detail/activity queries, worker enumeration, and measured compression semantics; verify empty, active, completed, failed, weighted, and period cases with tests.
- [x] 2.2 Add separate dashboard-token authentication, browser sessions, protected settings APIs, request limits, and CSRF protection; verify authorization, expiry, invalid input, worker isolation, and credential separation with handler tests.
- [x] 2.3 Document dashboard configuration, API payloads, metric boundaries, deployment, and capacity controls in operational and OpenSpec reference documents.

## 3. Dashboard interface

- [x] 3.1 Deliver embedded responsive overview, worker, and job views with useful SVG charts, live progress, period filtering, capacity editing, empty/error states, and login/logout; verify browser interactions and responsive layouts.

## 4. Integrated verification

- [x] 4.1 Run relevant Go tests with race detection, PostgreSQL integration checks, browser verification, and strict OpenSpec validation; review implementation against every delta requirement.

## Workflow follow-up

- Synchronize delivered contracts and archive this change after verification.
