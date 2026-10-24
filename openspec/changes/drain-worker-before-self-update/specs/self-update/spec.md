# Spec Delta

## MODIFIED Requirements

### Requirement: Update checks during service
The worker SHALL check for an update before requesting a new job when it is ready to accept work. If an update is available, it SHALL stop accepting new jobs, allow all accepted jobs to finish, and then restart so the supervisor can apply the update. The server SHALL check periodically while no HTTP requests are active.

#### Scenario: Server has active requests
- **WHEN** the server's periodic check is due but an HTTP request is active
- **THEN** it skips that check

#### Scenario: Worker is ready for work
- **WHEN** the worker reaches a job-request cycle and accepts new work
- **THEN** it checks for an update before asking for an assignment

#### Scenario: Worker discovers an update with jobs active
- **WHEN** the worker detects a newer release while one or more accepted jobs are still active
- **THEN** it stops requesting jobs, lets those accepted jobs finish, and requests the update restart only after the active job count reaches zero

#### Scenario: Supervisor receives an update restart request
- **WHEN** the worker child exits with the update-restart code
- **THEN** the supervisor refreshes release information immediately and applies the newer matching binary before starting another child
