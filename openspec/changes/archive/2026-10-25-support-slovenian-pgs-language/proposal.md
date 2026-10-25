# Proposal

## Why

PGS subtitles tagged with the ISO 639 bibliographic code `slo` currently reach PgsToSrt unchanged, which requests a Tesseract language file named `slo`. The worker image includes the Slovenian model as `slv.traineddata`, so OCR fails even though the required language data is present.

## What Changes

- Map `slo` (and the equivalent two-letter code `sl`) to Tesseract's `slv` model name.
- Add regression coverage for Slovenian language aliases.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: require Slovenian language aliases to select Tesseract's `slv` model.

## Impact

- Affects PGS subtitle language selection in `worker/step/PGStoSrtStep.go` and its unit tests.
- Updates the OpenSpec subtitles capability. No image dependency change is needed; the pinned tessdata revision already contains `slv.traineddata`.
