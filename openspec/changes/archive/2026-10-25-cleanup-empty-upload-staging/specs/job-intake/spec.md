# Spec Delta

## ADDED Requirements

### Requirement: Empty upload staging directories are cleaned up after finalization
After successful upload finalization, the server SHALL attempt to remove the `.transcoderd-upload` directory for staging files it removed, when that directory is empty. Cleanup failure SHALL NOT undo a published output or prevent the completed job from being recorded, and a non-empty staging directory SHALL remain unchanged.

#### Scenario: Destination-local staging directory becomes empty
- **WHEN** a completed job's staged upload is removed and no other files remain in its destination-local `.transcoderd-upload` directory
- **THEN** the server removes that empty directory

#### Scenario: Legacy root staging directory becomes empty
- **WHEN** a completed job's legacy staged upload is removed and no other files remain in the source-root `.transcoderd-upload` directory
- **THEN** the server removes that empty directory

#### Scenario: Another upload still uses the staging directory
- **WHEN** a completed job's staged upload is removed while another file remains in the same `.transcoderd-upload` directory
- **THEN** the server preserves the directory and the remaining file
- **AND** the completed job remains finalized
