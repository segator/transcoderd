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
A paused worker SHALL not request jobs. A worker SHALL stop requesting jobs when its active-job count reaches `worker.maxActiveJobs`, whose registered default SHALL be `2`. If a legacy time window is configured, the worker SHALL additionally request jobs only while local time elapsed since midnight lies between `startAfter` and `stopAfter`.

#### Scenario: Worker paused
- **WHEN** the worker is paused
- **THEN** it does not request another job

#### Scenario: Ordinary active-job limit
- **WHEN** no time window is configured and two jobs are active at the default limit
- **THEN** the worker does not request another job

#### Scenario: Custom active-job limit
- **WHEN** `worker.maxActiveJobs` is `4` and three jobs are active
- **THEN** the worker can request another job

#### Scenario: Limit within configured window
- **WHEN** local time is inside a configured interval but the active-job limit is reached
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** local time is outside the configured interval
- **THEN** the worker does not request a job

### Requirement: Bounded concurrent encoding
The worker SHALL bound simultaneous encoding jobs by the resolved encode limit while preserving a separate workspace, progress, and outcome for each job. `worker.ffmpegConfig.parallelJobs` SHALL default to `0`, meaning inherit `worker.maxActiveJobs`; a positive explicit value SHALL override that encode limit.

#### Scenario: Two encode jobs queued
- **WHEN** two accepted jobs reach the encode phase and the limit is `2`
- **THEN** both encoding jobs can run concurrently and each retains its own output and progress

#### Scenario: Encode limit reached
- **WHEN** the configured number of encoding jobs is already running
- **THEN** additional encode jobs wait until a slot is available

#### Scenario: Default encode concurrency follows admission
- **WHEN** `worker.maxActiveJobs` is `4` and the encode concurrency override is omitted
- **THEN** up to four accepted jobs can encode simultaneously

### Requirement: Weekly admission schedule
The worker SHALL accept optional `worker.schedule.windows` with default empty and `worker.schedule.timeZone` with default `Local`.

With weekly windows configured, the worker SHALL initiate job requests only inside at least one window in the selected timezone. It SHALL also respect pause and active-job limits.

#### Scenario: Request during an open window
- **WHEN** the worker is active, below its job limit, and inside a configured window
- **THEN** it may initiate the next normal job request

### Requirement: Daily CLI schedule shortcut
The worker SHALL accept repeatable top-level `--schedule=HH:MM-HH:MM` daily windows. These windows SHALL use the worker process's local timezone and replace YAML schedule windows when supplied.

#### Scenario: Weekday cutoff
- **WHEN** a Monday-to-Friday window ends at 10:00 in Europe/Madrid and the local clock reaches 10:00
- **THEN** the worker initiates no further job requests until another configured window opens

#### Scenario: Resume during another window
- **WHEN** the next configured interval opens and capacity is available
- **THEN** the next normal coordination poll can request another job

#### Scenario: Weekend override
- **WHEN** a Sat-Sun 00:00-24:00 interval is configured and the local day is Saturday
- **THEN** the schedule permits new requests throughout that day

#### Scenario: No weekly windows
- **WHEN** the window list is empty
- **THEN** the existing pause, limit, and optional legacy time window determine admission

### Requirement: Weekly window clock semantics
Window entries SHALL accept daily `HH:MM-HH:MM` or `Days@HH:MM-HH:MM`, with `*`, a three-letter weekday, or an inclusive weekday range as Days. Starts SHALL be inclusive and ends exclusive. Overnight windows SHALL belong to their starting weekday; `24:00` SHALL be accepted only as an end. The worker SHALL evaluate civil clock time using timezone transitions.

#### Scenario: Overnight interval
- **WHEN** a Friday 22:00-08:00 interval is configured and local time is Saturday 07:59
- **THEN** the interval permits a new request, but it stops permitting one at Saturday 08:00

#### Scenario: Daylight saving transition
- **WHEN** a timezone changes offset and a local civil time occurs in a configured interval
- **THEN** admission follows that civil time rather than elapsed seconds since midnight

#### Scenario: Daily command-line schedule
- **WHEN** `TZ=Europe/Madrid` and `--schedule=22:00-10:00` are supplied
- **THEN** the worker permits new requests daily from 22:00 inclusive until 10:00 exclusive in Europe/Madrid

#### Scenario: Repeated command-line schedule
- **WHEN** `--schedule=22:00-10:00 --schedule=14:00-16:00` are supplied
- **THEN** the worker permits new requests inside either daily period in its local timezone

#### Scenario: Command-line schedule overrides YAML
- **WHEN** YAML windows are configured and one or more `--schedule` flags are supplied
- **THEN** the worker uses the command-line windows instead of the YAML windows

### Requirement: Accepted work across closed windows
Accepted jobs SHALL continue after a window closes, including work accepted before a restart. A request initiated before the boundary MAY complete afterward. The schedule SHALL NOT perform automatic electricity-tariff or public-holiday lookups.

#### Scenario: Accepted jobs when admission closes
- **WHEN** a window closes while accepted jobs are encoding or waiting in the local pipeline
- **THEN** those jobs continue without schedule-driven suspension or cancellation

#### Scenario: Restored work outside a window
- **WHEN** the worker restores previously accepted jobs while admission is closed
- **THEN** those jobs can continue, while new requests remain blocked

#### Scenario: Request already in flight
- **WHEN** a job request is initiated before the cutoff and its assignment arrives afterward
- **THEN** the worker can process that assignment

### Requirement: Begin assigned work
A worker receiving an assignment SHALL create a per-job temporary directory, publish `Job/started` using the assignment's event sequence, and queue the download phase.

#### Scenario: Assignment received
- **WHEN** the worker receives a job `id` and `eventID`
- **THEN** it initializes the workspace, reports `Job/started`, and queues download
