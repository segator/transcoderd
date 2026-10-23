# Spec Delta

## MODIFIED Requirements

### Requirement: Versioned image tags
Publishing SHALL tag server and worker images with their role and requested version under `ghcr.io/segator/transcoderd`.

#### Scenario: Versioned publish
- **WHEN** both images are published for a version
- **THEN** `server-<version>` and `worker-<version>` tags are available

#### Scenario: Release aliases retain the canonical application version
- **WHEN** a release image is published under full, minor, and major version tags
- **THEN** all tags for each role resolve to the same image artifact built with the full semantic release version
