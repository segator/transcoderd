# Media transfer

## Purpose

Define how the assigned worker obtains the source, verifies its integrity, and returns an encoded file to the server.

## Requirements

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

### Requirement: Source checksum availability
The server SHALL calculate SHA-256 while streaming the source and SHALL expose the published checksum at `GET /api/v1/checksum?uuid=<id>` only after a complete source stream closes.

#### Scenario: Completed source stream
- **WHEN** the source has been streamed and its checksum published
- **THEN** the checksum route returns HTTP 200 with the hexadecimal SHA-256 text

#### Scenario: Checksum unavailable
- **WHEN** no checksum is available for the job source
- **THEN** the checksum route returns an error

#### Scenario: Source read fails
- **WHEN** the source stream stops before completion due to a read error
- **THEN** the server does not publish the incomplete stream's checksum

### Requirement: Worker download verification
The worker SHALL compare downloaded byte count with the declared content length and its locally calculated SHA-256 with the server checksum before probing the source. It SHALL retry eligible download failures, but SHALL not retry a source reported as HTTP 404.

#### Scenario: Download is intact
- **WHEN** both length and checksum match
- **THEN** the worker probes the downloaded media and continues

#### Scenario: Length or checksum differs
- **WHEN** the length or checksum does not match
- **THEN** the download attempt fails and the worker does not encode those bytes

#### Scenario: Source not found
- **WHEN** the download route returns HTTP 404
- **THEN** the worker treats that source as not found rather than retrying it

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

### Requirement: Completion after accepted upload
The worker SHALL hash the encoded file and send its checksum with the upload. It SHALL report `Job/completed` and clean its temporary job directory only after receiving HTTP 201.

#### Scenario: Upload accepted
- **WHEN** the server returns HTTP 201
- **THEN** the worker reports job completion and removes its temporary workspace

#### Scenario: Upload attempt rejected
- **WHEN** the server returns a response other than HTTP 201
- **THEN** that attempt does not cause a `Job/completed` notification

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

### Requirement: Download progress per attempt
Download progress SHALL describe bytes received in the current attempt rather than accumulating bytes across attempts.

#### Scenario: A new download attempt starts
- **WHEN** the worker starts a replacement download attempt
- **THEN** its reported transferred byte count resets to zero

### Requirement: Protect an unreadable source from repeated downloads
If a source read fails while serving an assignment, the server SHALL refuse subsequent downloads for that assignment without reopening the source. It SHALL return HTTP 404 to stop existing workers from retrying and log the source read failure. A new assignment SHALL be allowed to try again. The guard SHALL remain effective while the server process runs, up to 24 hours.

#### Scenario: Source read failure
- **WHEN** the server observes a source read error during a download
- **THEN** subsequent download requests for that assignment receive HTTP 404 without another source read

#### Scenario: Manual retry after repair
- **WHEN** a failed job is manually requeued and assigned again
- **THEN** the new assignment can attempt to read the source

#### Scenario: Client disconnect
- **WHEN** the client disconnects without a source read error
- **THEN** a later request for the assignment can retry the download
