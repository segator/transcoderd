## MODIFIED Requirements

### Requirement: PGS conversion and empty tracks
The worker SHALL produce an SRT for each selected PGS stream. It SHALL create an empty valid SRT when the `.sup` file is under 1024 bytes or the OCR tool reports zero displayable segments. The PGS bitmap decoder SHALL ignore decoded pixel writes outside the bitmap dimensions declared by the subtitle object. For OCR output, it SHALL correct cues whose end time precedes their start time.

#### Scenario: PGS with text
- **WHEN** OCR completes with subtitle items
- **THEN** an optimized SRT is available for encoding

#### Scenario: Empty PGS track
- **WHEN** the extracted track is too small or contains no displayable segments
- **THEN** an empty valid SRT is written and processing continues

#### Scenario: Oversized PGS bitmap run
- **WHEN** a decoded PGS RLE run extends beyond the dimensions declared for its bitmap
- **THEN** out-of-bounds pixels are discarded, in-bounds pixels remain available for OCR, and decoding does not fail with an index exception

#### Scenario: Reversed cue times
- **WHEN** an OCR cue ends before its start time
- **THEN** its end time is moved to two seconds after its start
