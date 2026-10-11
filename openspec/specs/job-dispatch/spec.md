# Job dispatch

## Purpose

Define how an available job is assigned to a worker, when a worker requests work, and how that worker begins execution.

## Requirements

### Requirement: Request an available job
The server SHALL serve `GET /api/v1/job/request` to a caller with a `workerName` header. An available queued job SHALL produce an assignment event and a response containing the job `id` and assignment `eventID`.

#### Scenario: Queued job exists
- **WHEN** a worker requests work and a queued job exists
- **THEN** the server records `Job/assigned` with that worker's name and returns HTTP 200 with `id` and `eventID`

#### Scenario: Queue is empty
- **WHEN** no job is queued
- **THEN** the API returns HTTP 204

#### Scenario: Missing worker name
- **WHEN** the `workerName` header is absent
- **THEN** the API returns HTTP 403

### Requirement: Single-server assignment ordering
The server SHALL choose the oldest queued job by its status event time and SHALL avoid assigning the same queued job twice when it receives concurrent requests in one server process.

#### Scenario: Two queued jobs
- **WHEN** a worker requests work while two jobs are queued with different event times
- **THEN** the job with the earlier queued event is assigned first

### Requirement: Periodic worker coordination
An active worker SHALL send a presence event approximately every five seconds and SHALL attempt a job request approximately every five seconds when it accepts new work.

#### Scenario: Worker remains active
- **WHEN** the coordination interval elapses
- **THEN** the worker sends a presence event and evaluates whether to request work

### Requirement: Local admission policy
A paused worker SHALL not request jobs. Without a configured time window, a worker SHALL stop requesting jobs when it has two active jobs. With a configured window, it SHALL request jobs only while local time elapsed since midnight lies between `startAfter` and `stopAfter`.

#### Scenario: Worker paused
- **WHEN** the worker is paused
- **THEN** it does not request another job

#### Scenario: Ordinary active-job limit
- **WHEN** no time window is configured and two jobs are active
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** local time is outside the configured interval
- **THEN** the worker does not request a job

### Requirement: Begin assigned work
A worker receiving an assignment SHALL create a per-job temporary directory, publish `Job/started` using the assignment's event sequence, and queue the download phase.

#### Scenario: Assignment received
- **WHEN** the worker receives a job `id` and `eventID`
- **THEN** it initializes the workspace, reports `Job/started`, and queues download
