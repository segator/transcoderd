# Design

## Context

The worker maps source subtitle language metadata to Tesseract model names in one helper. The pinned `tessdata` archive already includes `slv.traineddata`; only the bibliographic and two-letter aliases need normalization.

## Goals / Non-Goals

**Goals:**
- Normalize Slovenian subtitle language aliases before invoking PgsToSrt.
- Keep the change local to language selection and cover it with a unit test.

**Non-Goals:**
- Changing the worker image's tessdata source or adding language model dependencies.
- Changing OCR behavior for languages other than Slovenian.

## Decisions

- Map `slo`, `slv`, and `sl` to `slv`, matching the traineddata filename. This follows the existing alias table and avoids adding a separate file or changing PgsToSrt.
- Test the language-selection helper with a table of aliases and existing fallback cases rather than invoking OCR, which would require a large model file and a real subtitle stream.

## Risks / Trade-offs

- [A future tessdata revision could remove the model] → The pinned model file was verified at the current tessdata commit; the mapping test guards the selected model name.
