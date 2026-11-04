# Tasks

## 1. API-only backend
- [x] 1.1 Remove backend UI/assets serving, add authenticated session status and retain dashboard/media APIs; verify backend auth/media tests and document API boundaries.

## 2. Independent frontend
- [x] 2.1 Move UI/PWA assets to a standalone HTTP frontend with session checks and restricted streaming API proxy; verify authentication, CSRF, redirects, ranges, cancellation, upstream failure and PWA tests.

## 3. Runtime and distribution
- [x] 3.1 Add isolated CLI/configuration, binary/image/publication targets and two-host deployment docs; verify configuration precedence, builds and a container without media mounts/tools.

## 4. Integration
- [x] 4.1 Run full race/JavaScript/lint/spec checks and two-process Chromium checks for English UI, worker controls, library intake, playback/seeking and offline/auth behavior; rebuild the local preview and record evidence.

## Workflow follow-up
- Sync delta specs, archive the completed change and update PR #202 with independent frontend deployment and validation.
