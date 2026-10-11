# Configuration and runtime

## Purpose

Define how the server and worker receive configuration, start their dependencies, expose control access, and shut down. The full setting inventory and FFmpeg mapping are in `openspec/CONFIGURATION.md`.

## Requirements

### Requirement: Configuration inputs
Both binaries SHALL accept registered CLI flags and YAML configuration, and SHALL enable environment lookup with the `TR_` prefix. A supplied CLI flag SHALL override the corresponding file value.

#### Scenario: No configuration file
- **WHEN** no configuration file exists in the searched paths
- **THEN** the process logs a warning and continues with registered defaults

#### Scenario: CLI override
- **WHEN** a registered CLI flag and YAML specify different values for the same key
- **THEN** the value supplied on the command line is used

### Requirement: Shared web configuration
The server SHALL listen on `web.port`, which defaults to `8080`. Workers SHALL use `web.domain`, which defaults to `http://localhost:8080`, as the base URL for server requests. Control requests SHALL use the configured `web.token`, whose registered default is `admin`.

#### Scenario: Custom server port
- **WHEN** `web.port` is configured to a different port
- **THEN** the server listens on that port

#### Scenario: Worker base URL
- **WHEN** `web.domain` points to a remote server
- **THEN** the worker sends job and event requests to that base URL

### Requirement: Bearer protection for control routes
The server SHALL require `Authorization: Bearer <web.token>` on job creation, job cancellation, job requests, and worker events.

#### Scenario: Missing or incorrect token
- **WHEN** one of the four control routes receives no bearer token or a different token
- **THEN** the API returns HTTP 401

#### Scenario: Matching token
- **WHEN** a control route receives the configured bearer token
- **THEN** the request reaches that route's handler

### Requirement: Server startup dependencies
The server SHALL require an absolute `server.scheduler.sourcePath` and SHALL initialize PostgreSQL before starting the scheduler and HTTP API. The registered defaults for `sourcePath`, `scheduleTime`, `jobTimeout`, and `minFileSize` SHALL be `/data/current`, `5m`, `24h`, and 100,000,000 bytes.

#### Scenario: Relative media root
- **WHEN** the configured source root is relative
- **THEN** server startup fails before the HTTP API starts

#### Scenario: Database initialization failure
- **WHEN** PostgreSQL initialization fails
- **THEN** the scheduler and HTTP API do not start

#### Scenario: Successful startup
- **WHEN** the media root is absolute and database initialization succeeds
- **THEN** the scheduler starts and the API begins listening

### Requirement: Worker startup dependencies
The worker SHALL send an initial presence event before accepting jobs and SHALL create a workspace under `worker.temporalPath` named for `worker.name`. The registered defaults SHALL be the OS temporary directory and the host name.

#### Scenario: Initial presence fails
- **WHEN** the initial presence event cannot be published
- **THEN** job coordination does not start

#### Scenario: Initial presence succeeds
- **WHEN** the initial presence event succeeds
- **THEN** the worker prepares its workspace, execution pipeline, and server coordinator

### Requirement: Shutdown signals
Both processes SHALL respond to interruption or termination by canceling their service context and stopping their active services.

#### Scenario: Server termination
- **WHEN** the server receives a termination signal
- **THEN** it cancels scheduling and requests graceful HTTP shutdown

#### Scenario: Worker termination
- **WHEN** the worker receives a termination signal
- **THEN** it stops coordination, step executors, and console rendering
