## ADDED Requirements

### Requirement: Direct closed-caption conversion
The worker SHALL convert selected EIA-608 subtitle streams directly from the source to SRT in the Matroska output, preserving their language, title, forced and commentary metadata.

#### Scenario: Selected EIA-608 captions
- **WHEN** a selected subtitle is reported as `eia_608`
- **THEN** encoding converts it to SRT without external extraction or an unsupported codec copy

### Requirement: Valid converted subtitle inputs
The worker SHALL validate extracted and OCR-generated SRT before encoding. It SHALL preserve cue text and timing when normalizing recoverable serialization issues, explicitly represent empty converted tracks, and fail the subtitle phase with diagnostics for invalid nonempty tracks.

#### Scenario: Empty converted text track
- **WHEN** successful text conversion produces an empty subtitle track
- **THEN** the worker supplies a valid empty SRT input

#### Scenario: Recoverable OCR serialization
- **WHEN** OCR output has literal timing arrows inside dialogue or recoverable line-ending serialization issues
- **THEN** normalization retains its text and timing before existing cue optimization

#### Scenario: Invalid nonempty conversion
- **WHEN** converted text cannot be parsed as valid SRT
- **THEN** the subtitle phase fails before video encoding and reports the affected track
