# Spec Delta

## ADDED Requirements

### Requirement: Per-encode pause controls
The dashboard SHALL retain its general control and show an individual Pause/Resume button for each controllable active CPU/GPU encoding. It SHALL explain that individual controls pause in RAM, preserve individual holds across general Resume, and identify individual versus general holds. The service SHALL route job IDs to their owning profile and reject mutations during maintenance or pending reconfiguration.

#### Scenario: Pause and resume a job row
- **WHEN** the user pauses or resumes an active encoding from its row
- **THEN** the action reaches that job's owning profile and updates that row's pause state without changing the other profile or its limits

#### Scenario: General pause holds a row
- **WHEN** the general RAM pause holds an encoding
- **THEN** the row explains the general hold and disables its Resume button until the general pause is lifted

#### Scenario: Queued or completed job
- **WHEN** a job has no active encoding process
- **THEN** no individual active-encode control is offered and a stale direct request is rejected

#### Scenario: Mandatory update or pending configuration
- **WHEN** maintenance or deferred reconfiguration is active and a UI or direct request attempts an individual pause
- **THEN** the mutation is blocked so accepted work can drain
