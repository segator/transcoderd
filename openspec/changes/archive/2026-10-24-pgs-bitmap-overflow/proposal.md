# PGS bitmap overflow handling

## Why

Some PGS subtitle objects contain RLE runs that expand beyond their declared bitmap dimensions. The pinned PgsToSrt decoder writes these pixels without checking the bitmap bounds, causing an `IndexOutOfRangeException` for each affected cue. On the reported job, all 935 cues failed and PgsToSrt produced an empty SRT, causing the subtitle step to fail.

## What changes

Make the PGS bitmap decoder ignore pixel writes outside the allocated bitmap while preserving valid in-bounds pixels. Add a regression test for a one-pixel bitmap containing an oversized RLE run and run it as part of the PGS builder pipeline.

## Capabilities

- `subtitles`: safely convert PGS tracks containing oversized bitmap runs.

## Impact

The change is limited to PGS subtitle OCR. Video and audio processing are unchanged. Malformed pixels outside the dimensions declared by the PGS object are discarded; valid pixels and subtitle cues continue through OCR.
