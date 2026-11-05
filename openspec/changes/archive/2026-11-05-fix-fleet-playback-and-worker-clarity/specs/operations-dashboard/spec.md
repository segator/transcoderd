## MODIFIED Requirements

### Requirement: Visual media exploration and job playback
The dashboard SHALL offer authenticated folder navigation, selectable videos, editable presets, an embedded player and reviewed submission. Job rows SHALL label playback as queued source or completed output; active, failed and canceled jobs SHALL have no playback action. Scan/load progress, actual successes, skipped/failed reasons, cancellation, empty/error states and unavailable files SHALL be shown without invented data.

#### Scenario: Browse and play queued source
- **WHEN** an operator opens a queued job's available source
- **THEN** its video can be played inside the dashboard without worker assignment or a worker credential

#### Scenario: Active or failed job
- **WHEN** a job is active, failed or canceled
- **THEN** its row has no playback action and its job-assets endpoint rejects playback

#### Scenario: Review before loading jobs
- **WHEN** an operator changes selection filters
- **THEN** no jobs are added until a completed preview and explicit file selection are submitted

## ADDED Requirements

### Requirement: Filtered paginated jobs
The dashboard SHALL query all jobs by state and case-insensitive filename, worker or ID search before pagination. Pages SHALL contain at most 100 jobs with a matching total and navigation; the default page size SHALL be 25. Overview counters SHALL remain global.

#### Scenario: Completed jobs behind a large queue
- **WHEN** more than 100 queued jobs precede completed jobs
- **THEN** selecting Completed returns completed jobs with their matching total and further pages remain accessible

### Requirement: Recent fleet membership and offline presentation
Fleet counts and worker listings SHALL include only workers seen within the previous 30 days, independently of the selected statistics period. Online workers SHALL appear first, with active workers before idle peers. Worker lists SHALL paginate at 12 per page. Offline workers SHALL show basic identity and last-seen information without capacity controls or live resource details.

#### Scenario: Historical worker
- **WHEN** a worker has no heartbeat or was last seen over 30 days ago
- **THEN** it does not contribute to the fleet total or worker list

#### Scenario: Offline recent worker
- **WHEN** a worker was seen within 30 days but is currently offline
- **THEN** it appears after online workers as Offline without editable limits

### Requirement: Fixed worker profile controls
Fixed Windows CPU and GPU worker profiles SHALL identify their profile and shared host scope. Each SHALL expose only its supported encoder limit and relevant CPU or GPU utilization; independent workers SHALL retain both limits. Limits SHALL still be saved atomically with the unsupported class set to zero.

#### Scenario: Windows CPU profile
- **WHEN** an operator opens a managed CPU profile
- **THEN** CPU controls and CPU/RAM metrics are shown without GPU controls or GPU utilization

#### Scenario: Windows GPU profile
- **WHEN** an operator opens a managed GPU profile
- **THEN** GPU controls and GPU/RAM metrics are shown without CPU controls or CPU utilization

### Requirement: Signed-in operator identity
OIDC sessions SHALL retain verified name, preferred username, email and subject display claims inside the encrypted frontend cookie. An authenticated, non-cacheable identity response SHALL expose display identity without credentials or provider tokens. The dashboard SHALL display a name or username and email when available; token login SHALL identify a token session without inventing a user.

#### Scenario: Verified account
- **WHEN** a verified OIDC identity includes a name and email
- **THEN** the dashboard displays that name and email across frontend replicas

#### Scenario: Revoked session
- **WHEN** the API session has been revoked
- **THEN** the identity endpoint returns unauthorized even if the frontend cookie decrypts
