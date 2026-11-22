## MODIFIED Requirements

### Requirement: Assignment-checked download
`GET /api/v1/download?uuid=<id>` SHALL serve the source only while the job is `started` and the `workerName` header matches the assigned worker. Updated workers SHALL send the assignment identity and the server SHALL reject a stale identity. Its `Content-Disposition` filename SHALL use RFC 5987 UTF-8 encoding so workers can recover the original filename.

#### Scenario: Assigned worker
- **WHEN** the assigned worker requests a started job
- **THEN** the server returns HTTP 200 with file bytes, `Content-Length`, and `Content-Disposition` filename

#### Scenario: Different worker
- **WHEN** another worker requests the file without an assignment identity
- **THEN** the API returns HTTP 403

#### Scenario: Missing source file
- **WHEN** the source file of an otherwise eligible job no longer exists
- **THEN** the API returns HTTP 404

#### Scenario: Stale assignment identity
- **WHEN** a request identifies an expired assignment
- **THEN** the server explicitly rejects it even if the same worker name owns the new assignment

### Requirement: Assignment-checked upload
`POST` and `PUT /api/v1/upload?uuid=<id>` SHALL accept an output stream only from the worker assigned to a `started` job. Updated workers SHALL send their assignment identity, and the server SHALL recheck that identity before publishing the upload. The server SHALL compare the declared `Content-Length` and `checksum` header with the received byte count and calculated SHA-256 before returning HTTP 201.

#### Scenario: Valid output stream
- **WHEN** the assigned worker sends the declared number of bytes and matching SHA-256
- **THEN** the API returns HTTP 201

#### Scenario: Invalid size or checksum
- **WHEN** the actual byte count or SHA-256 differs from the declared value
- **THEN** the API returns HTTP 400

#### Scenario: Different worker
- **WHEN** a worker other than the assigned worker tries to upload without an assignment identity
- **THEN** the API returns HTTP 403

#### Scenario: Reassignment during upload
- **WHEN** an assignment expires before its upload commits
- **THEN** the server rejects the stale upload without publishing it

### Requirement: Bounded incomplete downloads
The worker SHALL stop after three source transfers that end cleanly before their declared size or three failed source integrity checks for one assignment and report an actionable failure. Ambiguous network interruptions SHALL be retried without an overall deadline. The server SHALL NOT automatically requeue a job with an actionable failure. Users MAY explicitly retry the failed job after resolving the cause.

#### Scenario: Repeated truncated transfer
- **WHEN** three source transfers end cleanly before their declared size
- **THEN** the worker fails the job with a diagnostic identifying repeated incomplete downloads and the server does not automatically requeue it

#### Scenario: Repeated verification failure
- **WHEN** three downloaded sources cannot be verified against the server checksum
- **THEN** the worker fails the job with a diagnostic identifying repeated verification failures and the server does not automatically requeue it

#### Scenario: Transient interruption
- **WHEN** a network interruption interrupts a transfer
- **THEN** the worker retries until the source is complete and verified, the assignment expires, or shutdown is requested
