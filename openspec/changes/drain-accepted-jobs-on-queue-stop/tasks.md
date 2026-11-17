# Tasks

## 1. Parked ownership and recovery

- [ ] 1.1 Add durable assignment generations, per-worker credentials, reproducible capabilities with shared versioned server keys, authenticated parked ownership, idempotent request keys and reconciliation, with explicit cancellation and safe recovery; test worker impersonation, key rotation and loss, timeouts, server replicas, lost request/park responses and concurrent resume/reassignment.
- [ ] 1.2 Persist and restore the recoverable next phase, encoder profile and prefetched data atomically; test legacy snapshots, missing/corrupt workspaces and restarting after a completed download without downloading again.
- [ ] 1.3 Fence and deduplicate worker notifications and progress by current generation, persist pending notifications locally and reconcile lost acknowledgements; test stale and duplicate events and crashes around acknowledgement.
- [ ] 1.4 Isolate uploads by generation and implement a durable, recoverable publication intent that serializes finalization with cancellation and reassignment; test uploads already streaming during revocation, duplicate completion, crashes at each filesystem/database boundary and safe source deletion.
- [ ] 1.5 Negotiate protected protocol capability without letting legacy transfer routes bypass fencing for protected assignments; test mixed server/worker versions and safe failure during upgrades or rollback.

## 2. Quiescence and queue controls

- [ ] 2.1 Track running phases, encodes, reservations and durable parked jobs without races; test in-flight assignment, phase transition, explicit RAM/individual holds and resume races.
- [ ] 2.2 Preserve stopped queue and parked jobs across restart, then resume parked work before admitting new assignments; test zero limits, CPU/GPU profiles, independent holds and settings outages.
- [ ] 2.3 Add a local bounded, cancellable safe-idle wait with a restart barrier that fails closed on unknown status or non-durable work; test pending I/O, suspended encodes, reservations, parked jobs, lost callers and concurrent resume.
- [ ] 2.4 Validate declared restart-persistent workspace identity and use renewable execution leases with fail-closed start and stop-on-lease-loss; test ephemeral/missing storage, changed volume identity, server outage and an unreachable stale worker.

## 3. Supervisors and operator surfaces

- [ ] 3.1 Make safe shutdown and applicable upgrades use the safe-idle gate without starting parked encodes when the queue is stopped; test restart, Windows CPU/GPU profiles and older workers with unknown state.
- [ ] 3.2 Display running, parked and safe-idle state in local and fleet interfaces; test telemetry compatibility and explain assigned-but-idle jobs and retained pause controls.
- [ ] 3.3 Update configuration, worker deployment, update, dashboard and protocol documentation for safe idle, workspace and server ownership requirements; verify CLI examples against the implementation.

## 4. Integration validation

- [ ] 4.1 Exercise active encode plus prefetched download through stop, safe idle, restart and resume, including server timeout, lost responses, stale-worker publication and crash recovery; run relevant Go tests and strict OpenSpec validation.

## Workflow follow-up

- Archive the completed change and verify the main specs match the delivered behavior.
