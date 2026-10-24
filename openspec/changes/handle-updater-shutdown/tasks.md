# Tasks

## 1. Supervisor shutdown

- [x] 1.1 Stop the updater loop and release its wait-group slot on context cancellation; verify the updater unit tests pass.
- [x] 1.2 Add a regression test for cancellation while the supervised child is running; verify the test observes a clean return without panic.
- [x] 1.3 Validate the change with `openspec validate --all --strict` and `go test ./update`.
