## ADDED Requirements

### Requirement: Read-only server queue view
The server SHALL provide an authenticated read-only queue view containing the total number of queued jobs and at most the oldest 100 job identifiers, source file names, sizes, and queue times. Reading the view SHALL NOT assign jobs or change their order.

#### Scenario: Inspect queued work
- **WHEN** an authorized client requests the queue view
- **THEN** it receives the queued count and up to 100 oldest jobs without changing their assignment state

#### Scenario: No queued work
- **WHEN** the queue is empty
- **THEN** the response contains a zero count and an empty job list

#### Scenario: Unauthorized request
- **WHEN** the queue view is requested without the correct bearer token
- **THEN** the server returns HTTP 401
