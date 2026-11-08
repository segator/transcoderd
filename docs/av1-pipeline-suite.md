# AV1 pipeline benchmark suite

The experiment selects a storage/time/quality trade-off for concurrent CPU and
GPU workers. The operator accepts a small visible difference in difficult
scenes when storage savings justify it. No default is changed by this suite.
The [2026-11-08 results](av1-pipeline-results.md) include the measured depth
curves, Windows batches, output integrity and original-resolution size checks.

`scripts/av1_pipeline_suite.py` measures independent factors:

| Factor | Alternatives |
| --- | --- |
| Output depth | 8 and 10 bits, including 8-bit and genuine 10-bit sources |
| Dimensions | Original versus the existing 1920-pixel width cap |
| GPU decode | Software decoding versus NVDEC |
| GPU filtering | CPU scale; CPU decode plus CUDA scale; NVDEC plus CPU scale; NVDEC plus CUDA scale; original dimensions with necessary format conversion only |
| Direct hardware input | NVDEC surfaces directly into NVENC, including its internal 8-to-10-bit conversion |
| Contention | Isolated timing; one CPU plus one GPU job; one CPU plus two GPU jobs; two CPU plus two GPU jobs |

CPU uses SVT-AV1 4.3.0 preset 4 / CRF26. GPU uses the production NVENC p7/UHQ
CQ38 profile, full-resolution multipass, middle B references, disabled split,
and QP range 0–100. Depth screening also measures CPU CRF24/26/28 and GPU
CQ36/38/40. Equal CRF/CQ values do not prove equal quality across depths or
encoders. Use measured curves to identify candidates, then confirm any
quality-matched recommendation with actual encodes; do not extrapolate curves.

## Input and controls

Use local, video-only stream copies of real H.264/HEVC/AV1/VP9 excerpts. Retain
their source origin and exact extraction command in a private manifest. Never
use FFV1 references for decode performance: those bypass the codec under test.
The manifest is a JSON array with `id` and absolute `source` fields. Optional
origin/probe fields are retained in the local source inventory.

Include dark gradients, grain, texture, movement and clean footage. Seek
coverage of 8-bit SDR, 10-bit SDR, 4K and HDR, documenting missing classes.
Synthetic samples prove functionality only. The short excerpt sweep cannot
establish full-film quality or library-wide savings.

Wait for production encodes to finish. Confirm Windows FFmpeg activity and
NVENC/NVDEC utilization before timing. Complete toolchain preparation first.
Record FFmpeg/build hashes, source hashes, CPU, memory and driver information.
Run quality metrics separately from all timed experiments.

## Execution

Use an FFmpeg build containing SVT-AV1 4.3.0, NVENC/NVDEC, `scale_cuda` and
`libvmaf`. Use a new output root when changing inputs, settings or tools:

```bash
python3 scripts/av1_pipeline_suite.py \
  --ffmpeg /lab/ffmpeg --ffprobe /lab/ffprobe \
  --manifest /lab/sources.json --output /lab/quality --phase decode
python3 scripts/av1_pipeline_suite.py \
  --ffmpeg /lab/ffmpeg --ffprobe /lab/ffprobe \
  --manifest /lab/sources.json --output /lab/quality --phase quality
python3 scripts/av1_pipeline_suite.py \
  --ffmpeg /lab/ffmpeg --ffprobe /lab/ffprobe \
  --manifest /lab/sources.json --scenes representative \
  --output /lab/performance --phase speed
python3 scripts/av1_pipeline_suite.py \
  --ffmpeg /lab/ffmpeg --ffprobe /lab/ffprobe \
  --manifest /lab/sources.json --scenes representative \
  --output /lab/performance --phase mixed
```

The quality sweep defaults to 360 frames per excerpt; repeated timings use
2400 frames and three repetitions with alternating execution order. Inspect
timing spread and confirm promising points on longer windows. Contention
runs sustain genuine looping SVT work while the finite GPU workload completes.
They record GPU throughput and overlapping CPU progress, rather than assuming
that faster GPU completion means more total work. CPU load holds the native
10-bit preset-4/CRF26 profile constant across GPU variants.

The full matrix requires `scale_cuda`; check the actual deployed binary rather
than inferring filter support from CUDA decoding support. The installed Windows
9.0.2 / SVT-AV1 4.3.0 bundle inspected on 2026-11-08 supports NVDEC and NVENC,
including NVENC's `-highbitdepth 1` internal conversion, but reports an unknown
`scale_cuda` filter. Native Windows confirmation therefore compares software
decode with and without the existing filter against direct NVDEC-to-NVENC,
using the installed bundle, original dimensions and 10-bit output. It uses
2400-frame jobs on both CPU and GPU, three alternating repetitions, and one or
two simultaneous SVT CPU jobs with two GPU jobs. All jobs start in the same
batch and all must finish. Batch elapsed time compares identical total work;
per-class finish times are retained separately. GPU and CPU fps in this native
report divide each class's fixed frame count by the whole batch duration, so
they sum to actual fixed-batch throughput. Outputs are retained and checked
after timing. Sustained-load overlap fps from WSL is a screening observation,
because different GPU runtimes can overlap different parts of a CPU clip.
This confirmation is separate from the complete CUDA-filter matrix under WSL.
`scripts/av1_pipeline_native.ps1` executes its private JSON plan: `directory`,
`ffmpeg`, `frames` and `rounds`; each round supplies `name`, `mode`, `repeat`,
and `loads`/`jobs` arrays containing `name`, Windows-quoted `arguments`, and
`progress` paths. Keep that generated plan private because it contains local
media paths. The runner starts and stops only its own child processes, retains
their logs and progress, and writes `native-results.json`. Use a fresh output
directory because FFmpeg is invoked with no-clobber output handling.

## Measurements and decision

- Require the requested output frame count, successful strict decoding,
  dimensions, depth and increasing video timestamps. Compare source CPU and
  NVDEC decoded-frame hashes separately before attributing quality changes.
- Retain exact command vectors, logs, repeated elapsed times and process/GPU
  resource samples. CPU percentage in Linux/WSL does not measure Windows host
  load; idle Windows counters must be checked separately.
- Record video packet bytes separately from MKV bytes. This suite initially
  isolates video; audio, OCR, subtitles, transfer and upload remain outside its
  timings and size comparisons.
- Score every excerpt frame with mean, p5, p1 and minimum VMAF and 10-bit SSIM.
  Preserve 10-bit metric input so quantizing a source to 8 bits is not hidden
  by reducing the reference to 8 bits before scoring. Use the 4K VMAF model for
  native 4K SDR; never compare scores across models as a common quality scale.
- Review native-size crops and clips at difficult scenes. Metrics alone cannot
  certify visually negligible loss or rule out banding.
- HDR receives no recommendation from SDR VMAF. A separate color-managed HDR
  assessment must establish whether reducing its depth is acceptable.
- Removing the cap from 4K increases the resolution processed by the encoder;
  report that separately from acceleration gains. Native 4K and capped 1080p
  are different outputs, not an equal-quality speed benchmark.
- Confirm the selected combination on a complete title and in the real worker
  pipeline before proposing any production behavior change through OpenSpec.

Raw private manifests, media, per-frame scores and resource samples belong in
ignored `build/av1-lab/` storage. A small, anonymized report may be committed
after results and limitations have been reviewed.
