# Encode failure fixes: November 8, 2026

## Audit scope

Production events from October 9 through November 8, 2026 contained 84 jobs with 105 failed attempts. Each attempt records both a failed phase and a failed job; those are not separate attempts. The initial snapshot showed 57 failed, 21 recovered, five queued and one running.

The operator authorized retrying RAM/VRAM failures after reducing concurrency. A fresh check found six still failed, nine already completed and two already queued. The six failures were requeued through the existing authenticated job API, without replaying completed work or changing concurrency.

## Delivered corrections

- **EIA-608:** selected closed captions now follow direct SRT conversion instead of unsupported Matroska stream copy. Mixed mov_text, extracted WebVTT and PGS inputs retain their independent indexes and language/disposition metadata.
- **Color matrix:** an identity/gbr matrix on known YUV input is cleared to unspecified on the encoded frames. Actual RGB input, valid SDR/HDR metadata and legacy saved contexts retain normal processing. Quality settings and per-job profiles are unchanged.
- **Extracted WebVTT:** whitespace-only separators caused successful FFmpeg conversions with zero output. Untimed annotation blocks could also stop conversion after only part of the track. Canonicalization keeps every timed cue, omits unplayable untimed blocks and verifies the converted cue count. Malformed timing blocks are rejected; cue times are never invented.
- **OCR SRT:** dialogue containing a literal timing arrow was misidentified as a timing header. The parser now distinguishes complete timing lines and protects dialogue arrows while preserving text, styling and timing. Rewrites replace the previous SRT only after successful serialization.
- **Retries:** explicit transient resource/network causes qualify within the existing attempt limit. Generic probe warnings, encoder-parameter hints and source stream listings no longer establish recoverability. Deterministic invalid input errors take precedence over incidental warnings.

## Evidence and validation

The original WebVTT extraction produced zero bytes with the installed Windows FFmpeg despite exit code 0. Normalization and checked conversion preserved all timed cues across its five tracks: **1484, 85, 1657, 199 and 1677**. The original OCR source reproduced the parser error at dialogue line 315; the corrected parser retained all **496** cues. Diagnostic sources remained outside the repository.

Synthetic real-tool tests generate a MOV with EIA-608 captions, run the generated AV1 command, inspect subtitle codec/language/dispositions, decode its text and fully decode the resulting video. Color fixtures exercise invalid YUV identity matrices, SDR and HDR, checking output matrix, transfer and primaries. Existing mov_text conversion and normal profile tests remain covered.

Local checks include all Go packages, race checks for affected worker/scheduler/context packages, Linux lint and strict OpenSpec validation. Real-tool regressions passed on Linux FFmpeg 9.0.2/SVT-AV1 4.2.0 and natively on Windows with the installed FFmpeg 9.0.2/SVT-AV1 4.3.0 bundle. Tests explicitly cover partial/empty converted output, malformed later cues, saved color metadata and the existing failure-limit boundary.

## Boundaries

These are complete subtitle-track and short generated-video checks, not newly completed full-film encodes or a quality benchmark. Unexplained native access violations/heap corruption/illegal instructions still require crash evidence and are not automatically labeled OOM. Historical bitmap-bounds, Slovenian-language and container/video-duration fixes already exist.

The 27 larger-output jobs remain rejected by the size guard at the operator's request. Possible grain/noise and content-specific profile adjustments need a separate measured quality/size investigation. No size/duration guard, encoder quality default, concurrency limit, API or database schema was changed.
