## MODIFIED Requirements

### Requirement: Classified failure retry
The server SHALL requeue failed jobs only for an explicit recoverable cause, including transient network and RAM/VRAM allocation failures. Generic codec-probe warnings, encoder-parameter hints and source stream listings SHALL NOT alone qualify. Deterministic invalid subtitle or color failures SHALL take precedence over incidental warnings. After more than ten `Job/failed` events, it SHALL cancel a recoverable job rather than retry it again.

#### Scenario: Recoverable failure
- **WHEN** a failed job has a classified recoverable cause and has not exceeded the failure limit
- **THEN** it receives a new `Job/queued` event

#### Scenario: Excessive failures
- **WHEN** a recoverable job has more than ten failed job events
- **THEN** it receives `Job/canceled` with the too-many-attempts reason

#### Scenario: Unclassified failure
- **WHEN** the failure message is not classified as recoverable
- **THEN** maintenance does not automatically requeue the job

#### Scenario: Incidental codec warning
- **WHEN** a failed encode log contains a codec-probe warning or generic encoder-parameter hint without an explicit transient cause
- **THEN** maintenance does not requeue it based on that warning

#### Scenario: Deterministic subtitle or color error
- **WHEN** an unsupported subtitle, invalid subtitle input or invalid color configuration causes failure alongside probe warnings
- **THEN** maintenance retains the failure rather than blindly repeating it

#### Scenario: Resource allocation failure
- **WHEN** encoding explicitly fails to allocate RAM or VRAM without a deterministic input failure
- **THEN** maintenance can requeue it within the existing failure limit
