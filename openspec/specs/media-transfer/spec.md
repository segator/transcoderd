# Media transfer

## Purpose

Define how the assigned worker obtains the source, verifies its integrity, and returns an encoded file to the server.

## Requirements

### Requirement: Assignment-checked download
`GET /api/v1/download?uuid=<id>` SHALL serve the source only while the job is `started` and the `workerName` header matches the assigned worker. Its `Content-Disposition` filename SHALL use RFC 5987 UTF-8 encoding so workers can recover the original filename.

#### Scenario: Assigned worker
- **WHEN** the assigned worker requests a started job
- **THEN** the server returns HTTP 200 with file bytes, `Content-Length`, and `Content-Disposition` filename

#### Scenario: Different worker
- **WHEN** another worker requests the file
- **THEN** the API returns HTTP 403

#### Scenario: Missing source file
- **WHEN** the source file of an otherwise eligible job no longer exists
- **THEN** the API returns HTTP 404

### Requirement: Source checksum availability
The server SHALL calculate SHA-256 while streaming the source and SHALL expose the published checksum at `GET /api/v1/checksum?uuid=<id>` once the stream closes.

#### Scenario: Completed source stream
- **WHEN** the source has been streamed and its checksum published
- **THEN** the checksum route returns HTTP 200 with the hexadecimal SHA-256 text

#### Scenario: Checksum unavailable
- **WHEN** no checksum is available for the job source
- **THEN** the checksum route returns an error

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
`POST` and `PUT /api/v1/upload?uuid=<id>` SHALL accept an output stream only from the worker assigned to a `started` job. The server SHALL compare the declared `Content-Length` and `checksum` header with the received byte count and calculated SHA-256 before returning HTTP 201.

#### Scenario: Valid output stream
- **WHEN** the assigned worker sends the declared number of bytes and matching SHA-256
- **THEN** the API returns HTTP 201

#### Scenario: Invalid size or checksum
- **WHEN** the actual byte count or SHA-256 differs from the declared value
- **THEN** the API returns HTTP 400

#### Scenario: Different worker
- **WHEN** a worker other than the assigned worker tries to upload
- **THEN** the API returns HTTP 403

### Requirement: Completion after accepted upload
The worker SHALL hash the encoded file and send its checksum with the upload. It SHALL report `Job/completed` and clean its temporary job directory only after receiving HTTP 201.

#### Scenario: Upload accepted
- **WHEN** the server returns HTTP 201
- **THEN** the worker reports job completion and removes its temporary workspace

#### Scenario: Upload attempt rejected
- **WHEN** the server returns a response other than HTTP 201
- **THEN** that attempt does not cause a `Job/completed` notification
