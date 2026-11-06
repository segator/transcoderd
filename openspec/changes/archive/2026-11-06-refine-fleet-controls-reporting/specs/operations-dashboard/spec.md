# Spec Delta

## MODIFIED Requirements

### Requirement: Useful fleet measurements
The dashboard SHALL show current queued, active, completed, failed, and canceled job counts; finalized input/output sizes and weighted storage savings; completion and failure history; worker statistics; and bounded recent jobs and activity. Period filters SHALL support 24 hours, 7 days, 30 days and custom date ranges. Terminal job counts SHALL use Job-level outcomes, not intermediate phase completion.

#### Scenario: Intermediate phase completion
- **WHEN** a job completes its FFMPEG phase but has not completed verification and upload
- **THEN** it remains active and its output does not contribute to finalized compression savings

#### Scenario: Weighted compression
- **WHEN** finalized jobs have valid source and target sizes
- **THEN** aggregate compression uses the sum of their sizes, and empty datasets show an empty state without fabricated measurements

### Requirement: Reporting-period worker visibility
The Workers view SHALL include only workers seen within its selected 24-hour, 7-day, 30-day or custom reporting period. Fleet-wide membership SHALL retain its 30-day maximum; custom-period visibility SHALL filter last-seen timestamps by both boundaries within that set, without claiming historical resource readings. Filtered worker counts and pagination SHALL describe the visible set.

#### Scenario: Last 24 hours
- **WHEN** Last 24 hours is selected and a worker was last seen two days ago
- **THEN** it is absent from Workers while a worker seen within 24 hours remains visible

#### Scenario: Custom range worker visibility
- **WHEN** a custom period ends before a worker's last-seen timestamp
- **THEN** that worker is absent from the period-filtered Workers list without altering live fleet totals

## ADDED Requirements

### Requirement: Reporting-range boundaries and scope
Custom ranges SHALL use start-inclusive/end-exclusive RFC3339 boundaries, spanning at most 3660 days. Invalid, incomplete, reversed or mixed preset/range requests SHALL be rejected. Period summaries, timelines, codec/worker outcomes and activity SHALL use both boundaries. Measured period source/output/saved bytes SHALL be separate from all-time totals; live jobs and fleet presence SHALL remain independent of outcome filters with explicit scope labels.

#### Scenario: Historical range savings
- **WHEN** an operator selects a past date range
- **THEN** completed/failed counts and finalized savings include only outcomes at or after its start and before its end, while live and all-time values remain separately labeled

#### Scenario: Invalid reporting range
- **WHEN** a range has a missing, invalid or reversed boundary, exceeds 3660 days or is combined with hours
- **THEN** it returns HTTP 400 without running a dashboard query


#### Scenario: API without custom-range support
- **WHEN** a custom-range response omits its boundaries or returns boundaries different from the request
- **THEN** the frontend shows an API upgrade error and retains the previous results with their previous period label

### Requirement: Individual remote encode control
Online worker cards SHALL show each current task's filename, phase, progress and pause state. Capable workers SHALL offer authenticated same-origin acknowledged task Pause/Resume. Commands SHALL validate UUID and fresh controllable ownership at submission and application, preserve unrelated holds, and report pending/failure. Older workers SHALL retain worker-wide controls without receiving individual commands.

#### Scenario: Pause one encode
- **WHEN** an operator pauses one of two active encodes
- **THEN** only the selected encode suspends, its progress is retained, and its Resume action appears after acknowledgement

#### Scenario: Resume under worker RAM hold
- **WHEN** an operator resumes an individually paused encode while the worker RAM hold remains active
- **THEN** only the individual hold clears and the encode stays suspended by the worker

#### Scenario: Ownership changes before application
- **WHEN** a queued command targets a job that finishes or leaves the worker before application
- **THEN** it fails explicitly without affecting another encode


#### Scenario: Invalid or unsupported individual request
- **WHEN** a request lacks a valid job UUID, targets another worker's encode or targets a version-1 worker
- **THEN** it is rejected without queuing individual control or modifying any job

### Requirement: Concise responsive operations layout
Navigation, login and section headings SHALL use functional names without promotional slogans. Overview SHALL group selected-period measurements separately from live/all-time measurements and identify the period scope. Mobile layouts SHALL use multiple columns for compact metrics and small resource/analytics charts when viewport width permits, while keeping task controls readable and avoiding page-level horizontal overflow.

#### Scenario: Mobile monitoring
- **WHEN** an operator opens Overview or worker resource details at a typical mobile width
- **THEN** compact metrics and compatible small charts share rows, larger task/control sections retain usable widths, and the page has no horizontal overflow
