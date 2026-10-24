# Design

## Context

The supervisor runs the application as a child process with the service context. On cancellation, Go terminates the child; the command runner can report this as an OS signal error rather than `context.Canceled`.

## Goals / Non-Goals

**Goals:** Treat context-driven child termination as normal shutdown, stop the updater loop, and complete the updater wait-group lifecycle.

**Non-Goals:** Change update checks, release selection, command-line flags, or worker job concurrency.

## Decisions

- Check the service context when the child command returns an error. Cancellation takes precedence over the child process's signal error; unrelated child failures remain fatal.
- Make the updater goroutine stop after cancellation and call `Done` exactly once through a deferred cleanup.
- Exercise cancellation with a helper child process in a regression test rather than mocking the command runner.

## Risks / Trade-offs

- A genuine child failure concurrent with shutdown is treated as shutdown. This is intentional because the service is already stopping; failures outside cancellation remain errors.
