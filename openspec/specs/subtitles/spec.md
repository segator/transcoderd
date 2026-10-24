# Subtitle handling

## Purpose

Define which subtitle streams the worker retains, which streams require extraction or OCR, and how converted text is included in the encoded Matroska file.

## Requirements

### Requirement: Subtitle stream selection
The worker SHALL retain forced and commentary subtitle streams and SHALL retain at least one ordinary stream for each source language. A selected subtitle whose format can be copied into the output SHALL be copied without text conversion.

#### Scenario: Ordinary language stream
- **WHEN** the source has an ordinary subtitle stream for a language
- **THEN** that language has a selected subtitle stream in the output

#### Scenario: Forced and commentary streams
- **WHEN** source subtitles are marked forced or commentary
- **THEN** those streams remain selected in addition to ordinary language streams

### Requirement: Identify streams requiring extraction
The worker SHALL extract PGS image subtitles and text streams whose detected format is empty, `none`, or `mov_text` before encoding. It SHALL skip the extraction phase when no selected subtitle needs it.

#### Scenario: PGS stream
- **WHEN** a selected subtitle format contains `pgs`
- **THEN** the worker extracts that stream as a `.sup` file for OCR

#### Scenario: Unsupported text codec
- **WHEN** a selected text stream is reported as empty, `none`, or `mov_text`
- **THEN** the worker extracts the stream and converts it to SRT

#### Scenario: No special subtitles
- **WHEN** no subtitle requires extraction
- **THEN** the worker advances from download directly to media encoding

### Requirement: OCR configuration
For each extracted PGS stream, the worker SHALL invoke PgsToSrt with the configured .NET path, assembly path, Tesseract data path, Tesseract version, and Leptonica settings. The registered defaults SHALL be `dotnet`, `./PgsToSrt.dll`, `./tessdata`, version `5`, `leptonica`, and version `6`.

#### Scenario: Default OCR tools
- **WHEN** no OCR tool paths or versions are overridden
- **THEN** the PGS conversion uses the registered tool locations and versions

#### Scenario: Configured OCR tools
- **WHEN** `worker.pgsConfig` provides different tool paths or versions
- **THEN** those values are passed to the PGS conversion

### Requirement: Parallel PGS conversion
The worker SHALL use `worker.pgsConfig.parallelJobs` as the number of concurrent OCR actions within a PGS phase. Its registered default SHALL be `1`.

#### Scenario: Multiple PGS streams
- **WHEN** a job has several selected PGS streams and `parallelJobs` is set to `2`
- **THEN** at most two OCR actions for that phase run concurrently

#### Scenario: Multiple PGS streams with the default
- **WHEN** a job has several selected PGS streams and no `worker.pgsConfig.parallelJobs` value is configured
- **THEN** at most one OCR action for that phase runs concurrently

### Requirement: OCR language selection
The worker SHALL derive the OCR language from the subtitle language metadata, mapping recognized language aliases to Tesseract names and using English when the source language is empty.

#### Scenario: Empty language
- **WHEN** a PGS stream has no language code
- **THEN** OCR uses `eng`

#### Scenario: Spanish language alias
- **WHEN** a PGS stream language is `es` or `esp`
- **THEN** OCR uses `spa`

### Requirement: PGS conversion and empty tracks
The worker SHALL produce an SRT for each selected PGS stream. It SHALL create an empty valid SRT when the `.sup` file is under 1024 bytes or the OCR tool reports zero displayable segments. For OCR output, it SHALL correct cues whose end time precedes their start time.

#### Scenario: PGS with text
- **WHEN** OCR completes with subtitle items
- **THEN** an optimized SRT is available for encoding

#### Scenario: Empty PGS track
- **WHEN** the extracted track is too small or contains no displayable segments
- **THEN** an empty valid SRT is written and processing continues

#### Scenario: Reversed cue times
- **WHEN** an OCR cue ends before it starts
- **THEN** its end time is moved to two seconds after its start

### Requirement: Extracted text conversion
A selected unsupported text stream SHALL be converted from the extracted file to SRT before the media encoding phase. Failure of that conversion SHALL fail the subtitle phase.

#### Scenario: Extracted text converts
- **WHEN** FFmpeg successfully converts an extracted text stream
- **THEN** its SRT becomes an additional input to media encoding

#### Scenario: Conversion fails
- **WHEN** conversion exits unsuccessfully
- **THEN** the subtitle phase fails and the job is not reported complete

### Requirement: SRT output metadata and dispositions
Converted SRT streams SHALL be included as separate encoding inputs. The worker SHALL carry their source language into the output, mark forced converted streams as forced and default, retain commentary disposition, and remove `PGS` from converted subtitle titles.

#### Scenario: Forced PGS converted
- **WHEN** a forced PGS stream is converted
- **THEN** its SRT output carries forced and default dispositions

#### Scenario: Commentary stream converted
- **WHEN** a commentary stream requires conversion
- **THEN** its output is marked as commentary

#### Scenario: PGS title
- **WHEN** a converted subtitle title contains `PGS`
- **THEN** the generated output title omits that label
