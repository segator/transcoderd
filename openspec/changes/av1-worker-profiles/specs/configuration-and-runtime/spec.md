# Spec Delta

## MODIFIED Requirements

### Requirement: Worker startup dependencies
The worker SHALL validate its encoder and concurrency settings before sending its initial presence event. It SHALL then create a workspace under `worker.temporalPath` named for `worker.name`. The registered defaults for those paths SHALL be the OS temporary directory and the host name.

#### Scenario: Invalid encoder configuration
- **WHEN** the selected encoder, its settings, or a concurrency limit is invalid
- **THEN** worker startup fails with a configuration error and it requests no jobs

#### Scenario: Initial presence fails
- **WHEN** the initial presence event cannot be published
- **THEN** job coordination does not start

#### Scenario: Initial presence succeeds
- **WHEN** configuration is valid and the initial presence event succeeds
- **THEN** the worker prepares its workspace, execution pipeline, and server coordinator
