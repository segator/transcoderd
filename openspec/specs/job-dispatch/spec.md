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

### Requirement: Local job admission limit
A worker SHALL admit at most its effective FFmpeg encode target plus one pipeline job, counting accepted jobs and in-flight reservations. Admission SHALL require a positive maximum and an open schedule. The default encode target SHALL remain two. Zero SHALL stop requests and new encodes; running encodes and downstream work SHALL finish. Lowering a positive target SHALL not interrupt active encodes.

#### Scenario: Ordinary active-job limit
- **WHEN** no time window is configured and three jobs are active or reserved at the default target
- **THEN** the worker does not request another job

#### Scenario: Custom active-job limit
- **WHEN** the effective encode target is `4` and four jobs are active
- **THEN** the worker can request another job

#### Scenario: Zero active-job limit
- **WHEN** the active-job maximum is set to zero at startup or runtime
- **THEN** the worker requests no new jobs and finishes running encodes and downstream work while retaining queued encodes

#### Scenario: Lower maximum while jobs are active
- **WHEN** an operator lowers the maximum below the number of active jobs
- **THEN** active jobs continue and the worker requests no jobs until capacity is available

#### Scenario: Raise maximum while worker is ready
- **WHEN** an operator raises the maximum above the active-job count
- **THEN** the worker may request jobs until the active-job count reaches the new encode target plus one

#### Scenario: Limit within configured window
- **WHEN** local time is inside a configured interval but active jobs and reservations have reached the encode target plus one
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** local time is outside the configured interval
- **THEN** the worker does not request a job

### Requirement: Runtime worker admission controls
Local controls SHALL persist operator limits before reporting success. Positive maxima SHALL set FFmpeg concurrency and resume queued encodes. Zero SHALL retain the positive target but stop requests and new encodes; running encodes and downstream work SHALL finish. Startup SHALL use server overrides for standalone workers and service-owned Windows limits. Maintenance draining SHALL finish accepted work without changing saved preferences. Controls SHALL remain local.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests and lets active encodes, verification and uploads finish, and holds waiting FFmpeg jobs until a positive maximum resumes them

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator sets a positive maximum different from the current value
- **THEN** the worker applies it to future job requests and FFmpeg encode concurrency without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of FFmpeg jobs already running
- **THEN** those encodes continue and no new encode starts until active encodes fall below the new target

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current maximum unchanged

#### Scenario: Paused encode slot becomes free
- **WHEN** an active encode finishes while paused and another accepted job is ready to encode
- **THEN** that queued job remains waiting until the user resumes

#### Scenario: Separate maintenance drain
- **WHEN** maintenance drains a paused worker for configuration or installation
- **THEN** all accepted work may finish while new assignments remain blocked and the saved pause preference remains intact

#### Scenario: Restore a persisted maximum after restart
- **WHEN** a standalone worker restarts with a persisted maximum for its configured worker name
- **THEN** it applies that value before requesting new jobs, even when its startup configuration specifies another maximum

#### Scenario: Use startup maximum when no override exists
- **WHEN** a standalone worker starts and the server has no persisted maximum for its configured worker name
- **THEN** it uses its configured startup maximum

#### Scenario: Server unavailable during persistence
- **WHEN** an operator changes the maximum while the server cannot persist the request
- **THEN** the local control reports an error and does not report the change as successful

### Requirement: Bounded concurrent encoding
The worker SHALL bound FFmpeg jobs by target and preserve per-job workspace, progress, and outcome. Startup target SHALL use positive `worker.ffmpegConfig.parallelJobs`, else `worker.maxActiveJobs` with minimum one for restored work. Positive runtime `set-max-jobs` SHALL replace the target. Each non-FFmpeg phase SHALL have one consumer.

#### Scenario: Two encode jobs queued
- **WHEN** two accepted jobs reach the encode phase and the limit is `2`
- **THEN** both encoding jobs can run concurrently and each retains its own output and progress

#### Scenario: Encode limit reached
- **WHEN** the configured number of encoding jobs is already running
- **THEN** additional encode jobs wait until a slot is available

#### Scenario: Runtime encode target overrides startup value
- **WHEN** an operator sets a positive max-jobs value different from the startup encode target
- **THEN** the FFmpeg target changes to that value while each non-FFmpeg phase continues to process one job at a time

#### Scenario: Default encode concurrency follows admission
- **WHEN** `worker.maxActiveJobs` is `4` and the encode concurrency override is omitted
- **THEN** up to four accepted jobs can encode simultaneously

#### Scenario: Desktop startup has stale server limits
- **WHEN** a Windows service worker starts with four configured GPU slots and a higher old server maximum
- **THEN** the service-owned limit is persisted before new requests, no more than four jobs encode at once, and restored work observes that limit

#### Scenario: Paused Windows startup
- **WHEN** the Windows service restarts with its saved pause preference
- **THEN** restored queued encodes do not start until Resume or maintenance draining

### Requirement: Weekly admission schedule
The worker SHALL accept optional `worker.schedule.windows` with default empty and `worker.schedule.timeZone` with default `Local`.

With weekly windows configured, the worker SHALL initiate job requests only inside at least one window in the selected timezone. It SHALL also respect the active-job maximum.

#### Scenario: Request during an open window
- **WHEN** the worker is active, below its job limit, and inside a configured window
- **THEN** it may initiate the next normal job request

### Requirement: Daily CLI schedule shortcut
The worker SHALL accept repeatable top-level `--schedule=HH:MM-HH:MM` daily windows. These windows SHALL use the worker process's local timezone and replace YAML schedule windows when supplied.

#### Scenario: Weekday cutoff
- **WHEN** a Monday-to-Friday window ends at 10:00 in Europe/Madrid and the local clock reaches 10:00
- **THEN** the worker initiates no further job requests until another configured window opens

#### Scenario: Next admission window opens
- **WHEN** the next configured interval opens and capacity is available
- **THEN** the next normal coordination poll can request another job

#### Scenario: Weekend override
- **WHEN** a Sat-Sun 00:00-24:00 interval is configured and the local day is Saturday
- **THEN** the schedule permits new requests throughout that day

#### Scenario: No weekly windows
- **WHEN** the window list is empty
- **THEN** the active-job maximum and optional legacy time window determine admission

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

### Requirement: Read-only server queue view
The server SHALL provide an authenticated read-only queue view containing the total number of queued jobs and at most the oldest 100 job identifiers, source file names, sizes, and queue times. Reading the view SHALL NOT assign jobs or change their order.

#### Scenario: Inspect queued work
- **WHEN** an authorized client requests the queue view
- **THEN** it receives the queued count and up to 100 oldest jobs without changing their assignment state

#### Scenario: No queued work
- **WHEN** the queue is empty
- **THEN** the response contains a zero count and an empty job list

#### Scenario: Unauthorized request
- **WHEN** the queue view is requested without the correct bearer token
- **THEN** the server returns HTTP 401
