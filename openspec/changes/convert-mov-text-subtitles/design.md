# Design

## Context

See proposal.md - Why. Subtitle classification currently treats `mov_text` as an extracted text codec. The FFmpeg generator then expects an auxiliary SRT input produced during the MKVExtract phase.

## Goals / Non-Goals

**Goals:**
- Map `mov_text` from source input 0 and encode it as SRT in the output MKV.
- Keep the MKVExtract and PGS OCR paths for formats that still require them.
- Preserve subtitle language, title, forced, and commentary metadata.

**Non-Goals:**
- Change OCR behavior, unsupported Matroska text extraction, subtitle selection, or global FFmpeg quality settings.

## Decisions

- `NeedsMKVExtraction` will identify only codecs requiring external extraction; `mov_text` will no longer be classified for that step.
- `FFMPEGGenerator.setSubtFilters` will map `mov_text` directly from its source stream index and select the SRT encoder. Other directly supported streams remain copied; PGS and extracted subtitle files remain mapped from auxiliary inputs.
- Add a build-tagged integration test that creates a short MP4 with a native MPEG-4 video stream and `mov_text` subtitle, runs the actual generated FFmpeg arguments, then probes and decodes the resulting SRT track. This verifies the command end to end without GPU hardware.
- Keep `mov_text` conversion inside the main encode so it uses the source timestamps and avoids adding a separate extraction/FFmpeg pass.

## Risks / Trade-offs

- [FFmpeg builds may not include the SRT encoder or MP4 `mov_text` support] → The integration test will fail with the configured test FFmpeg and report the codec error; the production FFmpeg builder already includes standard subtitle codecs.
- [Changing subtitle argument indexing could affect mixed subtitle sets] → Cover direct `mov_text` mapping alongside auxiliary PGS/extracted SRT inputs in unit tests, and keep the integration fixture focused on a real MP4 source.
