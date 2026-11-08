# Tasks

## 1. Compatible source encoding

- [x] 1.1 Convert selected EIA-608 to SRT; verify mixed auxiliary-input indexes, language/dispositions and real caption muxing.
- [x] 1.2 Preserve pixel-format/matrix probe metadata and conditionally clear invalid YUV identity matrices; verify SDR/HDR/RGB/legacy contexts and real encoding.

## 2. Subtitle validation and normalization

- [x] 2.1 Reproduce the extracted WebVTT/SRT failure, validate converted text including empty tracks, and capture tool diagnostics; verify regression fixtures and real-tool conversion.
- [x] 2.2 Normalize recoverable OCR SRT serialization without losing cue text/timing; verify malformed-input rejection and existing empty/reversed-cue behavior.

## 3. Causal retry classification

- [x] 3.1 Replace generic warning retries with explicit transient causes and deterministic precedence; verify mixed production-log regressions and unchanged maintenance attempt limit.

## 4. Integration and review

- [x] 4.1 Record audit discrepancies and validation boundaries; run Go suites, race checks, real-tool regressions, lint and strict OpenSpec validation.

## Workflow follow-up

- Archive this completed change and synchronize its delivered contracts.
- Create a PR, review the final diff, wait for required CI, and merge the validated head as authorized by the operator.
