# Design

## Context

`scripts/windows/build-ffmpeg.sh` shares `fetch` across six independent stages. Downloads already verify pinned SHA-256 values before extraction. CI run 42043641913 reached FFmpeg's download and failed on curl error 35 without retrying.

## Goals / Non-Goals

Recover temporary download failures and test the real shell helper with real curl. Preserve pinned inputs and fail closed on invalid bytes. Publishing and merging the release PR are outside this repair.

## Decisions

- Extract `fetch` into a sourced helper imported into the common Dagger base. This permits unprivileged tests without running compilers or writing build-wide absolute directories.
- Use `--retry 5 --retry-all-errors`, 30-second connection timeout, 120-second per-attempt maximum and 300-second retry window. These GETs are idempotent. Curl writes to a regular output file so partial data is discarded before a retry.
- Download into a unique temporary file in the build directory and clean it with an EXIT trap in a subshell. Verify SHA-256 before creating/extracting the source directory. A checksum mismatch fails immediately rather than retrying untrusted content.
- Test TLS connection reset before handshake, truncated response followed by success, wrong checksum, and exhausted transport retries with local servers. Tests use the production flags; the exhausted-retry case takes the real exponential backoff.

## Risks / Trade-offs

- Persistent failures take longer to surface → five retries, a bounded retry-start window and bounded individual requests.
- Changing a shared helper invalidates dependent cached stages once → keep unrelated source inputs separate, retaining the existing dependency DAG.
- Local tests need bash/curl/tar/sha256sum → detect missing tools explicitly; the Linux CI runner supplies them.
