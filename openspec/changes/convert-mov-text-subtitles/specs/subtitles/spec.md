# Spec Delta

## MODIFIED Requirements

### Requirement: Identify streams requiring extraction
The worker SHALL extract PGS image subtitles and text streams whose detected format is empty or `none` before encoding. It SHALL skip the extraction phase when no selected subtitle needs it.

#### Scenario: PGS stream
- **WHEN** a selected subtitle format contains `pgs`
- **THEN** the worker extracts that stream as a `.sup` file for OCR

#### Scenario: Unsupported text codec
- **WHEN** a selected text stream is reported as empty or `none`
- **THEN** the worker extracts the stream and converts it to SRT

#### Scenario: No special subtitles
- **WHEN** no subtitle requires extraction
- **THEN** the worker advances from download directly to media encoding

## ADDED Requirements

### Requirement: Direct mov_text conversion
The worker SHALL convert selected `mov_text` subtitle streams directly from the source media to SRT during FFmpeg encoding, without requiring the MKVExtract phase.

#### Scenario: MP4 mov_text stream
- **WHEN** a selected MP4 subtitle stream has format `mov_text`
- **THEN** FFmpeg maps the stream from the original input, writes it as SRT in the output MKV, and the job does not invoke MKVExtract for that stream

#### Scenario: mov_text-only subtitle handling
- **WHEN** the selected subtitles include `mov_text` and no PGS or other extractable subtitle codecs
- **THEN** the job proceeds from download to media encoding without a separate subtitle extraction phase
