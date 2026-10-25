# Spec Delta

## MODIFIED Requirements

### Requirement: OCR language selection
The worker SHALL derive the OCR language from the subtitle language metadata, mapping recognized language aliases to Tesseract names and using English when the source language is empty.

#### Scenario: Empty language
- **WHEN** a PGS stream has no language code
- **THEN** OCR uses `eng`

#### Scenario: Spanish language alias
- **WHEN** a PGS stream language is `es` or `esp`
- **THEN** OCR uses `spa`

#### Scenario: Slovenian language aliases
- **WHEN** a PGS stream language is `slo`, `slv`, or `sl`
- **THEN** OCR uses Tesseract's `slv` language data
