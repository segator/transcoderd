## ADDED Requirements

### Requirement: Protected assignment configuration
The server SHALL default `web.protectedEnabled` to false and require unique worker credential hashes and an active versioned signing key when enabled. A protected worker SHALL require an individual credential and an operator-provisioned persistent workspace UUID. Server replicas SHALL use a consistent keyring.

#### Scenario: Protected worker settings are supplied through the environment
- **WHEN** the worker receives `TR_WORKER_PROTECTED_CREDENTIAL` and `TR_WORKER_PROTECTED_WORKSPACE_ID`
- **THEN** it uses them as its credential and expected workspace identity

#### Scenario: Protected server setup is incomplete
- **WHEN** protected mode is enabled without valid unique worker hashes and a valid active signing key
- **THEN** server startup fails before exposing the protected protocol

#### Scenario: Workspace marker is missing or different
- **WHEN** the worker cannot verify the configured UUID against its existing persistent workspace marker
- **THEN** protected operation does not fall back to legacy assignments or report safe idle
