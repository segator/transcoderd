# Spec Delta

## MODIFIED Requirements

### Requirement: Parallel PGS conversion
The worker SHALL use `worker.pgsConfig.parallelJobs` as the number of concurrent OCR actions within a PGS phase. Its registered default SHALL be `1`.

#### Scenario: Multiple PGS streams
- **WHEN** a job has several selected PGS streams and `parallelJobs` is set to `2`
- **THEN** at most two OCR actions for that phase run concurrently

#### Scenario: Multiple PGS streams with the default
- **WHEN** a job has several selected PGS streams and no `worker.pgsConfig.parallelJobs` value is configured
- **THEN** at most one OCR action for that phase runs concurrently

