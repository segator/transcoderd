## ADDED Requirements

### Requirement: Multi-platform frontend image
The frontend image SHALL support Linux amd64 and Linux arm64. Each variant SHALL contain a static frontend executable for its declared architecture, a usable CA certificate bundle and the existing nonroot frontend entry point. Every published frontend version or alias SHALL identify one image index containing both variants built with the same application version. The exported frontend OCI archive SHALL retain both variants.

#### Scenario: Pull the frontend on either supported architecture
- **WHEN** a Linux amd64 or arm64 host pulls a frontend image tag
- **THEN** the registry provides the matching frontend runtime variant without requiring a different tag
- **AND** its frontend health endpoint is available independently of the API backend

#### Scenario: Export a frontend distribution image
- **WHEN** the frontend image archive is built
- **THEN** its OCI index contains Linux amd64 and arm64 variants with executables matching their declared architectures

#### Scenario: Validate frontend platform variants
- **WHEN** CI validates the frontend runtime images
- **THEN** both variants pass executable-architecture, application-version and HTTP health checks
- **AND** the exported OCI index contains both supported architectures
