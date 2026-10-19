# AV1 worker profile validation

Validated locally on 2026-10-19 with Go 1.25.10, FFmpeg 9.0.2 from the
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
| Changed-file credential scan | No secrets found; two matches were the E2E test's `%s` YAML placeholders |

The generated-command tests used the committed 10.12-second, 320×240 H.264 MKV
fixture with two AAC tracks and four PGS subtitle tracks. They supplied four
temporary SRT files to exercise the same FFmpeg stream mapping that follows
PGS OCR in a real job. Both encoded files contained AV1 10-bit video, two AAC
tracks, four SRT tracks, and a duration. The GPU test ran the generated
argument vector locally with the project FFmpeg binary. A separate Docker
smoke test ran the approved video profile inside the worker image with WSL2 GPU
access; that direct container test mapped one audio track and no subtitles.

The Docker end-to-end job used the CPU profile so CI does not need a GPU. Its
518,240-byte source became a 146,144-byte MKV (28.2% of source size); the
pipeline recorded the exact output size, retained English and Spanish audio
and subtitles, matched 10.12-second duration, and decoded the completed file.
That tiny synthetic fixture is a functional check, not an estimate for the
film library. CRF35 is an explicit test override, while the worker's default
CPU fallback is CRF23. GPU library quality remains content dependent; the
fixed profile does not promise p5 VMAF 91–92 on every scene.

To repeat the command-level tests with a suitable local FFmpeg build:

```bash
AV1_TEST_FFMPEG=/path/to/project/ffmpeg AV1_TEST_GPU=1 \
  go test -tags=integration -run TestGeneratedAV1CommandsEncodeAndDecode \
  ./worker/step
```

Omit `AV1_TEST_GPU=1` on a CPU-only host. The standard Docker E2E test is
`make test-e2e` after building both application images. The worker image's
GPU access instructions are in [the RTX 5080 validation](av1-nvenc-validation.md).
