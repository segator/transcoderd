# AV1 worker profile validation

Validated locally on 2026-10-19/20 with Go 1.25.10, FFmpeg 9.0.2 from the
versioned project builder, an RTX 5080, NVIDIA driver 617.42, and Docker
29.4.1. These checks validate command execution and pipeline behavior; the
quality and size decision for the default GPU profile comes from the
[watched full-film pilot](av1-gpu-full-film-command.md).

| Check | Result |
| --- | --- |
| `go test ./...` | Passed |
| `go test -race ./worker/...` | Passed |
| `devbox run -- golangci-lint run` | Passed with zero issues |
| Go server and worker binary builds | Passed |
| `openspec validate --all --strict` | 12 items passed |
| Generated AV1 command test with `AV1_TEST_FFMPEG` and `AV1_TEST_GPU=1` | CPU and GPU outputs encoded, probed, and fully decoded |
| Worker Docker image build | Passed with the existing versioned FFmpeg builder image |
| RTX 5080 encode inside worker image | Approved p7/UHQ/CQ34/middle-4/fullres/QP0..100 profile encoded; 10-bit AV1 output probed and fully decoded |
| Docker end-to-end test, SVT-AV1 preset 4 / CRF35 | Completed download, PGS OCR, AV1 encode, size/duration verification, upload, and final event |
| Docker end-to-end test, AV1 NVENC p7 / CQ34 on RTX 5080 | Completed the same pipeline with 10-bit AV1 and all seven video/audio/subtitle streams |
| Complete 89-minute film through Docker server and GPU worker | Passed with production AV1 video and `libfdk_aac` audio defaults; 21.022 GB became 1.493 GB |
| Changed-file credential scan | 38 text files reviewed; no credentials found. Two findings were a test constant reference and a documented password placeholder. |

The generated-command tests used the committed 10.12-second, 320×240 H.264 MKV
fixture with two AAC tracks and four PGS subtitle tracks. They supplied four
temporary SRT files to exercise the same FFmpeg stream mapping that follows
PGS OCR in a real job. Both encoded files contained AV1 10-bit video, two AAC
tracks, four SRT tracks, and a duration. The GPU test ran the generated
argument vector locally with the project FFmpeg binary. A separate Docker
smoke test ran the approved video profile inside the worker image with WSL2 GPU
access; that direct container test mapped one audio track and no subtitles.

The Docker end-to-end job used the CPU profile so CI does not need a GPU. Its
518,240-byte source became a 48,939-byte MKV (9.4% of source size) with the
production `libfdk_aac` audio default; the
pipeline recorded the exact output size, retained English and Spanish audio
and subtitles, matched 10.12-second duration, and decoded the completed file.
That tiny synthetic fixture is a functional check, not an estimate for the
film library. CRF35 is an explicit test override, while the worker's default
CPU profile is preset 4 / CRF23. GPU library quality remains content dependent; the
fixed profile does not promise p5 VMAF 91–92 on every scene.

An opt-in E2E run with `E2E_GPU=1` used the RTX 5080 inside the actual worker
container. The 518,240-byte fixture became a 50,414-byte AV1 MKV with FDK audio and passed
the same server, stream, full-decode, size, and event checks. This also checks
that the worker preserves the container's NVIDIA driver library path when it
starts FFmpeg. The former worker code appended `LD_LIBRARY_PATH=.` and hid the
WSL2 driver path from the child process; the AV1 worker now inherits the
container environment.

The current short application E2E is run with `mise exec -- mage test:e2e`.
It starts PostgreSQL, the server, and a CPU SVT-AV1 worker in Dagger, processes
the committed 10.12-second fixture, and verifies AV1 output, FDK AAC, converted
subtitles, size, duration, upload, and completion. The GPU and full-film
results below are retained as completed validation records from the earlier
Docker-based local harness; the current CI path exercises the short CPU job.

For a current encoder-only GPU smoke test, build the local worker image with
`mise exec -- mage docker:worker` and follow the WSL2 or Linux command in
[RTX 5080 validation](av1-nvenc-validation.md). This initializes NVENC and
encodes a short synthetic clip. A complete RTX 5080 application E2E was also
run during the original profile validation; its measured full-film result and
stream hashes are recorded below. CI does not claim GPU hardware coverage.

## Complete movie through the application

The full-film E2E run completed locally across 2026-10-19/20 (Europe/Madrid),
using PostgreSQL, the server, and the actual RTX 5080 worker container. It
submitted *La saga de Bruce Lee*, downloaded the source over the server API,
extracted PGS, ran OCR, encoded AV1 and both audio languages, verified the
output, uploaded it, and recorded the final completed event. The test then
probed and decoded the entire output successfully.

| Item | Measured result |
| --- | --- |
| Source | H.264 SDR, 1920×1080, 5,355.19 s, 21,022,090,356 bytes |
| Output | 10-bit AV1, Spanish and English AAC, one SRT, 5,355.23 s |
| Completed MKV | 1,492,848,730 bytes (1.493 decimal GB), 92.9% less than source |
| Worker download | 5 min 8 s |
| PGS extraction | 2 min 33 s |
| OCR | About 1 s (this source has only one subtitle cue) |
| FFmpeg encode | 15 min 23 s |
| Upload | About 2 s |
| Worker job, start to completion | 23 min 8 s |
| Entire E2E test, including setup, polling, output copy, and full decode | 27 min 28 s |

The output has the **exact byte count** of the earlier watched full-film
pilot, using the same video settings and `libfdk_aac -vbr 5` audio. Container
hashes differ, but [SHA-256 stream hashes](benchmarks/av1-full-film-stream-hashes.json)
match for the video, both audio tracks, and subtitle: the actual encoded stream
payloads are identical. A separate full decode with `-xerror`, `-map 0:v:0`,
and `-map '0:a?'` passed with no errors, covering both audio tracks. The retained
file is `build/av1-lab/plex/full-film-pilot/bruce/bruce-av1-worker-e2e.mkv`
in the primary local checkout. The input remained untouched. The subtitle
has no language/title tags because its source PGS stream had neither.

An initial complete E2E run passed with the harness's native-AAC override,
producing 1,505,472,725 bytes. The harness no longer overrides the audio codec:
both the tiny fixture and complete-film tests exercise the normal
`libfdk_aac` default. Both films passed a complete decode. The production
run is the result to use for deployment and size comparisons.

This validates one complete SDR 1080p job, including audio and subtitles.
It does not measure a complete CPU film, mixed CPU/GPU load, power use,
HDR preservation, or whole-film VMAF. The watched pilot's quality evidence
remains its visual review and six scored sample windows.

## Weekly admission follow-up

The worker now supports optional weekly request windows and an explicit IANA
timezone. Local checks on 2026-10-20 covered inclusive starts, exclusive ends,
multiple weekday windows, all-day weekends, overnight windows and weekday
rollover, UTC-to-local conversion, spring and autumn DST transitions, invalid
configuration rejection before server contact, and CLI-over-YAML precedence
(including clearing windows). Admission continues to respect pause and the
configured active-job limit. The coordinator rechecks admission after its update
check in case that check crosses a cutoff.

The rebuilt worker Docker image was started twice against a local recording
HTTP endpoint, using YAML `Europe/Madrid` windows. During a closed interval it
sent its startup presence and a periodic heartbeat but **zero job requests**.
During an open interval it sent both presence events and **one job request**,
which received HTTP 204. Both processes shut down normally. This confirms the
real binary/container uses the configuration and leaves presence reporting
active outside request windows. It is a control-flow check; it does not require
a GPU or score video quality. Deterministic boundary/DST tests use explicit
clock values rather than waiting for the wall clock to change.

Accepted and recovered jobs remain governed by the existing pipeline and keep
running after admission closes. The schedule does not suspend jobs, predict
completion times, classify public holidays, or retrieve electricity prices.
See [the scheduling reference](worker-scheduling.md) for examples and limits.

The follow-up also passed `go test ./...`, `go test -race ./cmd ./worker/...`,
`golangci-lint run` (zero issues), worker binary/image builds, and strict
OpenSpec validation (12 items). The rebuilt worker completed the CPU Docker
E2E in 17.80 seconds, including subtitle OCR, FDK-AAC, AV1, all seven streams,
full decode, upload, and completion events; the output remained 48,939 bytes.
A scan of all 43 changed text files against the PR base found no private keys,
service/GitHub tokens, or credential-bearing URLs. New scheduling files were
also reviewed for embedded credentials; none were found.
