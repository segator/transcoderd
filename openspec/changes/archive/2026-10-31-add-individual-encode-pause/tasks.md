# Tasks

## 1. Independent process pause policy

- [x] 1.1 Add job-specific holds, composed global transitions and maintenance resume-all; verify two real processes run/stop independently, global Resume preserves manual holds, stale IDs fail and cancellation/rollback remain correct.

## 2. Worker and dashboard controls

- [x] 2.1 Expose validated local/browser job controls and accurate job capabilities/reasons; verify authentication, stale jobs, unchanged limits and blocked maintenance controls.
- [x] 2.2 Route jobs to owning Windows profiles and extend tray/update locks; verify native Windows tests, fresh builds and no changes to other profile limits.
- [x] 2.3 Add row buttons with help, pending-request handling and clear hold reasons; verify JavaScript and real-browser interactions, global/manual composition and maintenance locks; update operational documentation.

## 3. Validation and preview delivery

- [x] 3.1 Run application race tests, native Windows process/control tests, Linux/Windows encoded-stream comparisons, lint and strict OpenSpec validation; record evidence.
- [x] 3.2 Build an updated preview installer, verify its payload and integrity and place it in Downloads for the user.

## Workflow follow-up

- Archive the completed change and verify the synchronized main specs.
