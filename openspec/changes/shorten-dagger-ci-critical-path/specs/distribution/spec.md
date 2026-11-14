## MODIFIED Requirements

### Requirement: Full-payload native Windows release validation

Windows release publication and existing-tag completion SHALL pass the unattended upgrade and recovery scenarios with fixtures containing the complete media and OCR payload. Manual validation without a tag SHALL also use the complete payload. Ordinary PR validation and main-branch pushes that do not publish a release MAY use the media tools and OCR data required by the fixture with the same installer and native upgrade scenarios.

#### Scenario: Ordinary CI validation

- **WHEN** the workflow validates a pull request or a main-branch push that does not publish a release
- **THEN** the full portable worker and desktop installer pass native validation and the focused update fixtures pass native upgrade and recovery validation

#### Scenario: Signed release validation

- **WHEN** a Windows release is published or an existing release is completed
- **THEN** native upgrade and recovery validation with the complete fixture payload succeeds before the workflow uploads signed Windows release metadata

#### Scenario: Manually validate complete fixtures

- **WHEN** the workflow is manually dispatched without a release tag
- **THEN** the native upgrade and recovery scenarios run against the complete fixture payload
