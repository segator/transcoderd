# Tasks

## 1. Fix terminal progress redraw

- [x] 1.1 Configure active trackers so repeated updates redraw in place; add a focused regression test that captures multiple console renders and verifies one active line is reused.
- [x] 1.2 Verify completed and failed trackers leave the active display, and run the relevant console and worker tests.

## 2. Validate the change

- [x] 2.1 Run `openspec validate --all --strict` and confirm the implementation matches the `events-and-progress` delta.
- [x] 2.2 Compile a smoke harness from the changed worker source and verify its progress output redraws in a TTY-backed Docker container without interrupting production jobs.
