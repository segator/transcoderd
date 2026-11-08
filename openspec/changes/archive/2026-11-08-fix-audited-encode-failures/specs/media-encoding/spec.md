## ADDED Requirements

### Requirement: Invalid source color matrix normalization
The worker SHALL clear an identity RGB color matrix incorrectly attached to YUV source video before encoding to the selected 4:2:0 profile. It SHALL preserve valid source color metadata, including HDR transfer and primaries, and SHALL retain existing quality, scaling and verification settings.

#### Scenario: Identity matrix on YUV input
- **WHEN** YUV source video incorrectly declares an identity RGB color matrix
- **THEN** the encoded frames use an unspecified matrix instead of passing that invalid declaration to the encoder

#### Scenario: Valid SDR or HDR metadata
- **WHEN** source video declares a valid color matrix, transfer function and primaries
- **THEN** the worker preserves those values without applying the invalid-matrix fallback

#### Scenario: RGB input
- **WHEN** actual RGB source video declares an identity matrix
- **THEN** the worker preserves normal RGB-to-YUV conversion rather than treating the source declaration as invalid
