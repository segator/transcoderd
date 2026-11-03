# Spec Delta

## ADDED Requirements

### Requirement: Visual media exploration and job playback
The authenticated dashboard SHALL offer folder navigation, selectable video results, editable filter presets, an embedded player and explicit reviewed job submission. Job rows SHALL offer playback of available sources and completed outputs. The UI SHALL show scan/load progress, actual successes, skipped/failed reasons, cancellation, empty/error states and unavailable files without invented data.

#### Scenario: Browse and play queued source
- **WHEN** an operator opens a queued job's available source
- **THEN** its video can be played inside the dashboard without worker assignment or a worker credential

#### Scenario: Review before loading jobs
- **WHEN** an operator changes selection filters
- **THEN** no jobs are added until a completed preview and explicit file selection are submitted
