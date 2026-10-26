# Spec Delta

## ADDED Requirements

### Requirement: Independent CPU and GPU worker profiles
The Windows dashboard SHALL let users configure separate encoder and concurrent-job limits for CPU and GPU profiles under one service.

#### Scenario: Mixed CPU and GPU configuration
- **WHEN** the user configures one CPU encode and four GPU encodes
- **THEN** the service runs both profiles concurrently with those independent limits

#### Scenario: Disable one profile
- **WHEN** the user sets one profile's concurrency to zero and the other to a positive value
- **THEN** the service accepts jobs only through the enabled profile

#### Scenario: Migrate a saved single-profile configuration
- **WHEN** an existing installation has one saved encoder and concurrency limit
- **THEN** the service maps that configuration to its matching CPU or GPU profile and preserves its behavior

### Requirement: Shared queue assignment across encoder profiles
CPU and GPU profiles SHALL request work independently from the existing server queue, and each assigned job SHALL use the encoder profile that received it.

#### Scenario: Both profiles request queued work
- **WHEN** CPU and GPU profiles have available capacity and jobs are queued
- **THEN** either profile may receive each job according to which request the server handles first

### Requirement: Aggregated mixed-profile monitoring
The dashboard SHALL aggregate active jobs, progress, encode counts, and admission state across enabled CPU and GPU profiles while identifying each job's profile.

#### Scenario: Display simultaneous mixed-profile work
- **WHEN** CPU and GPU profiles encode jobs at the same time
- **THEN** the dashboard shows both jobs and their respective profiles and includes both in aggregate counts

#### Scenario: Pause and resume mixed profiles
- **WHEN** the user pauses or resumes the worker
- **THEN** new assignments stop or resume for both profiles while already accepted work continues
