## ADDED Requirements

### Requirement: Windows desktop installer distribution
The release pipeline SHALL build a Windows desktop installer from the same worker and bundled tools as the portable ZIP and SHALL publish it only after native installation, service, desktop control and CPU worker validation succeed.

#### Scenario: Release installer
- **WHEN** a release passes Windows native validation
- **THEN** the release contains a versioned Windows installer in addition to the existing portable assets

#### Scenario: Installer validation fails
- **WHEN** installation, service control, tray smoke test or Windows CPU E2E fails
- **THEN** the Windows installer is not published as a validated release asset
