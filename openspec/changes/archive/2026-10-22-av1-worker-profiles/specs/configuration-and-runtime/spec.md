# Spec Delta

## MODIFIED Requirements

### Requirement: Worker startup dependencies
The worker SHALL validate its encoder, concurrency, and admission-schedule settings before sending its initial presence event. It SHALL then create a workspace under `worker.temporalPath` named for `worker.name`. The registered defaults for those paths SHALL be the OS temporary directory and the host name.

#### Scenario: Invalid encoder configuration
- **WHEN** the selected encoder, its settings, or a concurrency limit is invalid
- **THEN** worker startup fails with a configuration error and it requests no jobs

#### Scenario: Initial presence fails
- **WHEN** the initial presence event cannot be published
- **THEN** job coordination does not start

#### Scenario: Initial presence succeeds
- **WHEN** configuration is valid and the initial presence event succeeds
- **THEN** the worker prepares its workspace, execution pipeline, and server coordinator

#### Scenario: Invalid admission schedule
- **WHEN** the timezone or a window is invalid, or weekly windows are combined with active legacy startAfter/stopAfter settings
- **THEN** worker startup fails with a configuration error before contacting the server
