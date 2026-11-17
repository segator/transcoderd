## ADDED Requirements

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

## MODIFIED Requirements

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
