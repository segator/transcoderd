## ADDED Requirements

### Requirement: Full-payload native Windows release validation

Windows release publication and existing-tag completion SHALL pass the unattended upgrade and recovery scenarios with fixtures containing the complete media and OCR payload. Ordinary PR validation MAY use the media tools and OCR data required by its fixture with the same installer and native upgrade scenarios.

#### Scenario: Ordinary PR validation

- **WHEN** the workflow validates a pull request
- **THEN** the full portable worker and desktop installer pass native validation and the focused update fixtures pass native upgrade and recovery validation

#### Scenario: Signed release validation

- **WHEN** a Windows release is published or an existing release is completed
- **THEN** native upgrade and recovery validation with the complete fixture payload succeeds before the workflow uploads signed Windows release metadata
