# Spec Delta

## MODIFIED Requirements

### Requirement: Aggregated mixed-profile monitoring
The dashboard SHALL aggregate active jobs, progress, encode counts, and admission state across enabled CPU and GPU profiles while identifying each job's profile.

#### Scenario: Display simultaneous mixed-profile work
- **WHEN** CPU and GPU profiles encode jobs at the same time
- **THEN** the dashboard shows both jobs and their respective profiles and includes both in aggregate counts

#### Scenario: Pause and resume mixed profiles
- **WHEN** the user pauses or resumes the worker
- **THEN** new assignments and queued FFmpeg starts stop or resume for both profiles, while running encodes and downstream work finish

#### Scenario: Configured limits govern startup
- **WHEN** the desktop specifies one CPU and four GPU encodes and the server retains different old limits
- **THEN** both workers use the desktop limits before starting encodes or accepting assignments, with at most one additional pipeline job per enabled profile
