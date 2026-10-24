# Design: PGS bitmap overflow handling

The decoder already parses PGS RLE data into an ImageSharp pixel span. Its `PutPixel` helper currently indexes the span directly. Change that helper to write only when the index is within the span bounds. This keeps malformed runs from throwing while ensuring no pixel can be written outside the declared bitmap.

TranscoderD builds PgsToSrt from a pinned upstream commit in the Dagger `builder-pgs` pipeline. Apply a small tracked patch after fetching that source. Copy a small C# regression test into the upstream source tree and run `dotnet test` before publishing the PgsToSrt binaries. This keeps the fix reproducible and ensures a future source or patch mismatch fails the builder.

Validation also reruns the exact 38 MB SUP track extracted from the reported job. Before the patch, all 935 items failed and produced an empty SRT; after the patch, all 935 items are converted with no per-item decoder errors.
