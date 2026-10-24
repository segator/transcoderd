# Design

## Context

See proposal.md - Why. The worker renders local progress with `go-pretty/v6/progress` and currently sets `RemoveOnCompletion` on its trackers. The renderer uses this flag when calculating how many active lines to rewind before drawing the next frame, so enabling it prevents cursor movement over the active trackers.

## Goals / Non-Goals

**Goals:**
- Allow terminal redraws to replace the current active tracker lines.
- Keep completed trackers out of the live progress display.

**Non-Goals:**
- Change server progress event cadence or payloads.
- Change the Docker worker's TTY setting or job scheduling.

## Decisions

- Configure active trackers so the progress writer includes them when calculating cursor movement. Keep tracker lifecycle cleanup enabled through the writer's completion behavior, and confirm that completed entries disappear after the change.
- Preserve the existing progress library and terminal rendering architecture. The observed issue comes from the tracker option interacting with the library's cursor-rewind calculation; replacing the renderer would broaden a localized fix.
- Validate the user-visible behavior with a focused console regression test that captures multiple renders, plus the existing console and worker test suites.

## Risks / Trade-offs

- [The progress library's completion flag has non-obvious cursor semantics] → Cover both repeated active updates and completion cleanup in the regression test.
- [Non-interactive log consumers may display terminal control sequences differently] → Keep the behavior requirement scoped to interactive terminals; retain the existing `-t` worker launch configuration for Docker logs.
