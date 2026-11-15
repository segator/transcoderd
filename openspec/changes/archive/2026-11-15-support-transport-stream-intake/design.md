## Context

Direct job submissions and reviewed dashboard submissions use separate extension checks. The dashboard already accepts MPEG transport streams.

## Goals

Bring the two submission routes into agreement for `.ts` and `.m2ts`. Validate the API change and exercise the worker with a copy of a source from the library.

## Decisions

Extend the existing API allowlist and test the HTTP route with both extensions. The same discovery routine handles individual files and directories. Its size check, job persistence, and destination naming already apply to any admitted file.

## Risks and Trade-offs

Source files can contain subtitle tracks that require container-specific handling. Probe the library sample before running the encode and verify the resulting Matroska output. Run the sample in an isolated queue with its own copy of the source.
