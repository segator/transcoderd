# Design

## Context

The HTTP response advertises the source length before streaming. A source read error after headers causes an incomplete response indistinguishable from some transport failures. Download retries currently reuse the progress tracker and may repeat 180 times. The server can requeue some failed jobs based on message text.

## Goals / Non-Goals

**Goals:** Bound repeated incomplete responses without encoding partial bytes; retain recovery from a brief interruption and truthful per-attempt counters.

**Non-Goals:** Diagnose or repair physical disks, distinguish storage failures from network failures, change the protocol, or deploy the fix.

## Decisions

Count incomplete HTTP response bodies and failed integrity checks separately per assignment; stop after three failures in either category, even if other retryable errors occur between them. Return a stable diagnostic that server maintenance treats as permanent, with manual retry available. Other eligible failures retain their existing policy. Reset the tracker at the beginning of each request. Test the real HTTP transfer path using a local server that truncates its response and a shortened test retry delay.

## Risks / Trade-offs

- Three network interruptions can require an operator retry even when storage is healthy. This bounded cost prevents repeated multi-gigabyte transfers and the message asks the operator to check source storage, local storage and network.
- The legacy server does not recognize the new failure marker if the underlying error contains a retryable phrase. Updated server and worker provide the full guarantee; mixed versions require care.
