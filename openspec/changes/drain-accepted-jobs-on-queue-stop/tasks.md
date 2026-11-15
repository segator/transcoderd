# Tasks

## 1. Parked ownership and recovery

- [ ] 1.1 Add durable, authenticated parked-assignment ownership and reconciliation, with fencing, explicit cancellation and safe recovery; test timeout beyond the configured interval, concurrent resume/reassignment and lost acknowledgements.
- [ ] 1.2 Persist and restore the recoverable next phase, encoder profile and prefetched data atomically; test legacy snapshots, missing/corrupt workspaces and restarting after a completed download without downloading again.

## 2. Quiescence and queue controls

- [ ] 2.1 Track running phases, encodes, reservations and durable parked jobs without races; test in-flight assignment, phase transition, explicit RAM/individual holds and resume races.
- [ ] 2.2 Preserve stopped queue and parked jobs across restart, then resume parked work before admitting new assignments; test zero limits, CPU/GPU profiles, independent holds and settings outages.
- [ ] 2.3 Add a local bounded, cancellable safe-idle wait that fails closed on unknown status or non-durable work; test pending I/O, suspended encodes, reservations, parked jobs and concurrent resume.

## 3. Supervisors and operator surfaces

- [ ] 3.1 Make safe shutdown and applicable upgrades use the safe-idle gate without starting parked encodes when the queue is stopped; test restart, Windows CPU/GPU profiles and older workers with unknown state.
- [ ] 3.2 Display running, parked and safe-idle state in local and fleet interfaces; test telemetry compatibility and explain assigned-but-idle jobs and retained pause controls.
- [ ] 3.3 Update configuration, worker deployment, update, dashboard and protocol documentation for safe idle, workspace and server ownership requirements; verify CLI examples against the implementation.

## 4. Integration validation

- [ ] 4.1 Exercise active encode plus prefetched download through stop, safe idle, restart and resume, including server timeout and non-duplication; run relevant Go tests and strict OpenSpec validation.

## Workflow follow-up

- Archive the completed change and verify the main specs match the delivered behavior.
