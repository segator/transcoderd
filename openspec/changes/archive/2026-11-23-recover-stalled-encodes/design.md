## Approach

Start a per-attempt watchdog only after process startup. Monitor monotonic frame and output timestamp progress, resetting the inactivity clock while paused. After 15 minutes terminate the process tree using the existing cancellation mechanism. Retry the encode once, then report a clear failure to release the slot. Honor parent cancellation and the existing hardware decoding fallback.

## Trade-offs

An encoder legitimately producing no progress for 15 minutes may be restarted. A bounded retry prevents a pathological input from indefinitely monopolizing a worker.
