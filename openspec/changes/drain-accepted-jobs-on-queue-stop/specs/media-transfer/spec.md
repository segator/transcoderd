## MODIFIED Requirements

### Requirement: Assignment-checked download
`GET /api/v1/download?uuid=<id>` SHALL serve the source only while the job is `started` and the `workerName` header matches the assigned worker. For protected assignments, the server SHALL authenticate the current assignment generation before streaming. Its `Content-Disposition` filename SHALL use RFC 5987 UTF-8 encoding so workers can recover the original filename.

#### Scenario: Assigned worker
- **WHEN** the assigned worker requests a started job
- **THEN** the server returns HTTP 200 with file bytes, `Content-Length`, and `Content-Disposition` filename

#### Scenario: Different worker
- **WHEN** another worker requests the file
- **THEN** the API returns HTTP 403

#### Scenario: Missing source file
- **WHEN** the source file of an otherwise eligible job no longer exists
- **THEN** the API returns HTTP 404

### Requirement: Assignment-checked upload
`POST` and `PUT /api/v1/upload?uuid=<id>` SHALL accept an output stream only from the worker assigned to a `started` job. The server SHALL compare the declared `Content-Length` and `checksum` header with the received byte count and calculated SHA-256 before returning HTTP 201. For protected assignments, the server SHALL authenticate the generation and revalidate it when accepting the staged object.

#### Scenario: Valid output stream
- **WHEN** the assigned worker sends the declared number of bytes and matching SHA-256
- **THEN** the API returns HTTP 201

#### Scenario: Invalid size or checksum
- **WHEN** the actual byte count or SHA-256 differs from the declared value
- **THEN** the API returns HTTP 400

#### Scenario: Different worker
- **WHEN** a worker other than the assigned worker tries to upload
- **THEN** the API returns HTTP 403

#### Scenario: Assignment revoked during upload
- **WHEN** an upload starts while its assignment is valid but cancellation or reassignment wins before the upload is accepted
- **THEN** the server rejects the stale upload and does not publish it as the current assignment's staged or final output

#### Scenario: Legacy transfer against protected assignment
- **WHEN** a client omits generation credentials for a protected assignment
- **THEN** legacy transfer routes reject the request rather than bypassing assignment protection
