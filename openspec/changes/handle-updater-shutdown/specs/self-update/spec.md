# Spec Delta

## ADDED Requirements

### Requirement: Graceful supervisor shutdown
The update supervisor SHALL stop its supervised process and finish its lifecycle without panicking or launching another process after the service context is canceled.

#### Scenario: Service receives a shutdown signal
- **WHEN** the service context is canceled while the supervised process is running
- **THEN** the supervisor stops the child, exits its update loop, and releases its wait-group slot without panicking
