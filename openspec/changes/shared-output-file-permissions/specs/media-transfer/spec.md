## ADDED Requirements

### Requirement: Shared permissions on completed output
Before recording a job as completed, the server SHALL set the published output file mode to `0664` (owner and group read/write; others read-only). If the permission update fails, the server SHALL NOT record the job as completed or delete the source.

#### Scenario: Output is finalized
- **WHEN** a verified uploaded output is published at its final destination
- **THEN** the final file has mode `0664` before the job is recorded as completed

#### Scenario: Permission update fails
- **WHEN** the server cannot set the published output mode
- **THEN** the job is not recorded as completed and the source file is preserved
