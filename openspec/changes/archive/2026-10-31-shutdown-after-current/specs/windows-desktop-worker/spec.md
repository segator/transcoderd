# Spec Delta

## ADDED Requirements

### Requirement: Explicit shutdown after accepted work
Windows Stop after current SHALL ask whether to shut down after accepted work. Only a positive answer SHALL arm a one-time, nonpersistent shutdown request. Both profiles SHALL finish accepted work including queued encodes and uploads before shutdown, with new admission closed. Ordinary pause, idle, scheduling and maintenance SHALL NOT arm shutdown.

#### Scenario: User chooses shutdown
- **WHEN** the user answers Yes to the Stop after current shutdown question
- **THEN** both profiles stop accepting work, finish accepted jobs and in-flight requests, and the service requests normal Windows shutdown once after both report idle

#### Scenario: User declines
- **WHEN** the user answers No
- **THEN** ordinary Stop after current applies without a shutdown request

#### Scenario: Cancel or lifecycle transition
- **WHEN** the queue is resumed, configuration or maintenance begins, a worker exits or cannot report status, or the service restarts
- **THEN** the shutdown request is canceled and cannot fire from a later idle state

#### Scenario: Pending shutdown and failed shutdown
- **WHEN** shutdown is armed or Windows rejects the shutdown request
- **THEN** the dashboard shows the pending action with Resume queue cancellation or the error, and no automatic repeat shutdown occurs
