# Design

## Context

The server generates a provisional upload path when it accepts a job. The worker already verifies the encoded target before upload, but the server currently records completion without probing the received file. The server image includes FFmpeg and FFprobe binaries, so it can inspect the uploaded bytes itself.

## Goals / Non-Goals

**Goals:**

- Derive the final destination label from the uploaded file's actual video stream.
- Persist the final path and size before recording a job as completed.
- Keep sources and pre-existing destination files safe from overwrite.
- Preserve provisional upload naming, audio-label normalization, and source deletion behavior after successful validation.

**Non-Goals:**

- Trust a worker-reported codec label instead of inspecting the received output.
- Change FFmpeg encoder settings, job transfer paths, or `deleteOnComplete` behavior.
- Rename outputs for jobs that were already completed before this change.

## Decisions

- Keep the existing intake name as a provisional path. New upload bytes are written to a hidden, job-specific staging file under the provisional destination's parent directory, so the upload and final destination share a filesystem. For compatibility with uploads already staged in the server-root `.transcoderd-upload/<job-id>.upload` directory, copy them to a destination-local staging file before finalizing. Staging paths depend only on a validated job UUID and the validated target's parent directory, not on arbitrary user-provided paths. After a worker reports successful upload and completion, run FFprobe against the staged file and use the actual stream codec as the source of truth rather than worker configuration or source filename.
- Map FFprobe `av1` to the filename label `av1`, `hevc` to the library's conventional label `x265`, and `h264` to `x264`. For other detected video codecs, use their normalized FFprobe codec name as the label.
- Derive the final basename from the original source basename: replace a recognized video codec token with the detected output label, or add the label when the source has no recognized video codec token. Preserve audio-label normalization to `AAC` and the `.mkv` extension. Match codec tokens at filename boundaries.
- If the final destination equals the source, or is already occupied by another file, never overwrite it. Generate deterministic alternatives by appending `_encoded`, then `_encoded-2`, `_encoded-3`, and so on until a free path is found. Create the final path with no-clobber semantics so an existence check cannot race with a concurrent file creation; an ordinary name collision must never leave a valid job incomplete.
- Probe the staged file, create the final path without clobbering, and persist the final `target_path` and target size before adding the completed job event. The finalizer SHALL handle a legacy staging file on another filesystem by copying it to a destination-local temporary staging file before reserving the final name. Preserve the source unless finalization succeeds and `deleteOnComplete` is enabled. Make finalization safe to retry if event persistence or the worker's completion notification is retried; roll back or reconcile a filesystem move if the database update fails.

Alternatives considered:

- Using a configured or worker-reported codec label would describe intended settings, not necessarily the bytes that arrived. Probing the received file verifies what will actually be stored and needs no worker protocol change.
- Renaming only when the source has a codec suffix would leave untagged sources without an output label. Deriving the final name from the source plus probed output handles both tagged and untagged names consistently.

## Risks / Trade-offs

- FFprobe may fail on a corrupt or unsupported output → do not mark the job completed or delete its source; include the probe error in the job failure reason.
- The codec-derived path may already exist → preserve the existing file and advance the numbered fallback until a free name is atomically reserved.
- Filesystem rename and database update cannot share one transaction → make the operation idempotent and roll back or recover the file rename if persistence fails.
- Container overlay staging and Unraid bind-mounted media directories can be different filesystems → stage new uploads next to their destination and migrate legacy staged uploads through a destination-local temporary file before no-clobber finalization.
- Codec token matching can alter title text → use token-boundary matching and positive and negative filename tests.

## Migration Plan

1. Deploy the server version with output probing and final-path persistence.
2. No configuration change is required. Newly completed jobs are named from the codec detected in their uploaded output, including jobs already queued before deployment.
3. Already completed jobs keep their existing names unless separately reprocessed.
4. Roll back by deploying the previous server version; outputs already renamed remain at their persisted final paths.

## Open Questions

None.
