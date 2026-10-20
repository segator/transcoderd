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
| Docker end-to-end test, SVT-AV1 preset 3 / CRF35 | Completed download, PGS OCR, AV1 encode, size/duration verification, upload, and final event |
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
CPU fallback is CRF23. GPU library quality remains content dependent; the
fixed profile does not promise p5 VMAF 91–92 on every scene.

An opt-in E2E run with `E2E_GPU=1` used the RTX 5080 inside the actual worker
container. The 518,240-byte fixture became a 50,414-byte AV1 MKV with FDK audio and passed
the same server, stream, full-decode, size, and event checks. This also checks
that the worker preserves the container's NVIDIA driver library path when it
starts FFmpeg. The former worker code appended `LD_LIBRARY_PATH=.` and hid the
WSL2 driver path from the child process; the AV1 worker now inherits the
container environment.

To repeat the command-level tests with a suitable local FFmpeg build:

```bash
AV1_TEST_FFMPEG=/path/to/project/ffmpeg AV1_TEST_GPU=1 \
  go test -tags=integration -run TestGeneratedAV1CommandsEncodeAndDecode \
  ./worker/step
```

Omit `AV1_TEST_GPU=1` on a CPU-only host. The standard Docker E2E test is
`make test-e2e` after building both application images. The worker image's
GPU access instructions are in [the RTX 5080 validation](av1-nvenc-validation.md).

The same E2E test can use WSL2 GPU access and, optionally, a complete local
film without copying its large source. `E2E_MOVIE_PATH` points to the input;
`E2E_WORK_DIR` must be on the same filesystem so the test can hard-link it
into a disposable server media directory. The input itself is not modified.
`E2E_KEEP_OUTPUT` copies the final MKV to a chosen path before cleanup.

```bash
E2E_GPU=1 E2E_WORK_DIR=/path/to/large/local/workdir \
  E2E_MOVIE_PATH=/path/to/input.mkv \
  E2E_KEEP_OUTPUT=/path/to/review-output.mkv \
  E2E_SERVER_IMAGE=ghcr.io/segator/transcoderd:server-test \
  E2E_WORKER_IMAGE=ghcr.io/segator/transcoderd:worker-test \
  go test -count=1 -tags=integration -v -timeout 120m \
    -run TestDockerE2E ./integration
```

For the short GPU fixture test, set only `E2E_GPU=1` and the image variables.
This WSL2 mode mounts `/dev/dxg` and `/usr/lib/wsl/{lib,drivers}` into the
worker. On ordinary Linux, use the NVIDIA Container Toolkit when deploying
the worker; the opt-in WSL2 test helper is not a portable GPU runtime setup.

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
