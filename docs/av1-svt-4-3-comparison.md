# SVT-AV1 4.2.0 versus 4.3.0

This experiment compares the two library versions with the TranscoderD
pre-adjustment CPU profile: FFmpeg 9.0.2, preset 4, CRF25, planar 10-bit 4:2:0 and the existing
1920-pixel width cap. It is an offline film-excerpt comparison. NVENC, audio
policy remain outside the experiment. The version-only CRF25 measurements
are separate from the operator-selected CRF26 default described below.

## Measured result and integration decision

Measured on 2026-11-04 on the Ryzen 9 9900X3D with 24 logical CPUs under WSL2.
The operator paused production encodes before testing. The retained Windows
FFmpeg processes had unchanged CPU counters across samples, GPU encoder use
was 0%, and all toolchain builds finished before the timed runs. No other
encode or metric pass overlapped a timed encode.

Each CRF25 point below has **five sequential repetitions**; wall time is their
median. Video MB are decimal video packet bytes, excluding container overhead.

| Scene | SVT | Median seconds | Video MB | Mean VMAF | p5 VMAF | p1 VMAF |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1800 s | 4.2.0 | 5.178 | 2.905 | 92.204 | 89.990 | 89.180 |
| 1800 s | 4.3.0 | 4.806 | 2.961 | 92.184 | 89.872 | 89.183 |
| 3600 s | 4.2.0 | 5.879 | 9.926 | 92.723 | 86.415 | 81.101 |
| 3600 s | 4.3.0 | 5.499 | 10.003 | 92.714 | 86.448 | 81.352 |
| 5400 s | 4.2.0 | 4.938 | 1.987 | 90.382 | 86.593 | 83.270 |
| 5400 s | 4.3.0 | 4.538 | 2.027 | 90.354 | 86.783 | 83.390 |

At the unchanged profile, 4.3.0 reduced time by **7.17%, 6.46% and 8.09%**
respectively. Video bytes increased **1.94%, 0.77% and 2.00%**. The sum of
the three median times fell 7.19%; their total video payload increased 1.16%.
Run standard deviations were 0.016–0.140 seconds for 4.2.0 and 0.033–0.065
seconds for 4.3.0; all individual timings are retained in the CSV.

Mean VMAF decreased 0.009–0.029 point. p5 changes were -0.117, +0.033 and
+0.191; p1 improved in all three scenes. SSIM increased by 0.000082–0.000274
and PSNR by 0.018–0.024 dB. The metrics therefore show small mixed differences,
not identical output or a uniform quality loss. Both versions still fall below
the older p5 89–90 pilot target on the two difficult late-film excerpts.

Native-size center crops were inspected at each scene's midpoint and the frame
with the largest negative VMAF change. The worst per-frame changes were -0.372,
-0.687 and -0.700. Both versions visibly reduce some original grain; the
inspected crops show no obvious additional blocking, ringing or banding from
4.3.0. This limited still-frame review cannot rule out differences elsewhere
or establish temporal/full-film perceptual equivalence.

The Linux and Windows recipes now select **SVT-AV1 4.3.0**, with Linux builder
tag `builder-ffmpeg-9.0.2-av1-svt-4.3.0`. The measured CPU throughput benefit,
compatibility checks and small quality changes support that integration while
retaining preset 4 in the version-only test. After reviewing the CRF26 results,
the operator selected **preset 4 / CRF26** as the CPU default. Its 14.3–16.0%
byte reduction comes with lower measured quality and must not be attributed
to the library upgrade alone. Publication of the builder remains part of the
normal release process.

## Comparison at similar measured quality

### Selected smaller-file profile: preset 4 / CRF26

The measured 4.3.0 / CRF26 points reduce video payload compared with the
original 4.2.0 / preset-4 / CRF25 baseline:

| Scene | Video byte change | Mean VMAF change | p5 VMAF change |
| --- | ---: | ---: | ---: |
| 1800 s | -14.26% | -0.286 | -0.355 |
| 3600 s | -16.01% | -0.306 | -0.439 |
| 5400 s | -16.01% | -0.363 | -0.233 |

This is a measured size/quality trade-off, larger than merely recovering the
0.8–2.0% byte increase at CRF25. New center-crop comparisons pair the old CRF25
output with the new CRF26 output at the worst per-frame VMAF difference; these
are retained locally as `crf26-vs-old25-scene-{1,2,3}.png`. The difficult crops
show some additional smoothing/texture loss, so the lower score cannot be
assumed invisible. The operator approved CRF26 as the smaller-file CPU default.
Explicit CRF25 overrides remain available; legacy saved CRF0 profiles resume
at CRF25 and newly pinned profiles store their resolved CRF. Changing preset is
a separate speed/analysis-effort choice and has not been measured on 4.3.0 here.

### Interpolated quality matches

Both versions were also measured at CRF24 and CRF26 with three repetitions per
point. The following are **in-range interpolated estimates**, using linear
metric/time interpolation and logarithmic byte interpolation between adjacent
measured CRFs. They are not additional measured encodes at fractional CRFs.
The baseline is the measured 4.2.0 / CRF25 point in each scene.

| Scene | Estimated 4.3.0 CRF at matched mean VMAF | Time reduction | Video byte change | Remaining p5 change |
| --- | ---: | ---: | ---: | ---: |
| 1800 s | 24.931 | 7.06% | +3.50% | -0.090 |
| 3600 s | 24.973 | 6.47% | +1.36% | +0.049 |
| 5400 s | 24.925 | 7.84% | +3.96% | +0.229 |

Matching p5 instead produces different trade-offs:

| Scene | Estimated 4.3.0 CRF at matched p5 | Time reduction | Video byte change | Remaining mean change |
| --- | ---: | ---: | ---: | ---: |
| 1800 s | 24.702 | 6.71% | +8.86% | +0.065 |
| 3600 s | 25.070 | 6.48% | -0.50% | -0.030 |
| 5400 s | 25.450 | 8.06% | -6.53% | -0.179 |

Matching one metric does not match all metrics; the apparent byte savings at
matched p5 in the last scene accompany a lower mean. Neither table is a
recommendation to retune the worker: its CRF setting is integer-valued. The
short-scene evidence supports a modest time improvement at similar measured
quality, with content-dependent byte costs. It does not confirm a universal
10–11% wall-time reduction or unchanged quality on every title.

The [raw points and timings](benchmarks/av1-svt-4-3.csv),
[comparison/estimate JSON](benchmarks/av1-svt-4-3-comparison.json) and
[toolchain/reference fingerprints](benchmarks/av1-svt-4-3-toolchains.json)
are retained alongside this report. Full per-frame scores, commands, logs,
outputs and review PNGs remain under the ignored worktree lab directory.

## Method and evidence

`mage build:svtComparison` exports two matched Linux toolchains under
`build/av1-lab/svt-4-3/tools/{4.2.0,4.3.0}/`. Both start from the same production
builder and rebuild SVT with its release/static settings, then reconfigure and
relink the same checksum-pinned FFmpeg source with the original configure
arguments. Every unrelated installed dependency is shared. The original
4.2.0 version/checksum entry must match exactly before the override is applied.

The official SVT 4.3.0 archive has SHA-256
`95ce8fc2381ba707e65ac8b9b2e165915bfbe9cd4d2f7b0f58d64ce31bd98f6d`;
4.2.0 uses `c7b13c4a84bd3751aa35fcc72be13e6875467e7c2216879251a486e5b1e4e740`.
The Linux FFmpeg GitHub n9.0.2 archive retains its existing SHA-256
`6e374ed621e48faa40639307dff48ba6fe574a509977956d2cce9669b7cc27e9`.
Windows retains its separate official FFmpeg release archive/hash.

Three retained FFV1 references sample the same SDR film at 1800, 3600 and
5400 seconds. Each is 24 seconds, 1920x800 at 24 fps, with 576 decoded frames.
The source SHA-256 is
`c44401580284f2bf47fc37fffb0ed44f31a3e62ca453f5ceeef977bab897d181`.
The references were copied into this worktree and the original source was
only read. Its hash was independently checked against the retained extraction
provenance. The excerpts include night dialogue, dark interiors, daylight motion and visible
film texture; three excerpts from one film do not represent an entire library.

The references are 8-bit `yuv420p`. Both encoders receive the same 10-bit
conversion; that conversion does not recover additional source precision.
Quality analysis uses one fixed FFmpeg/model, explicit 8-bit `yuv420p`
conversion, VMAF `vmaf_v0.6.1` and every frame, plus SSIM and PSNR.
PSNR is calculated from aggregate 8-bit per-frame MSE; a perfect match has no
finite PSNR and is represented by JSON null. VMAF percentiles use nearest rank.
The worst sliding one-second mean retains temporal frame order.

For each scene/CRF there is one warm-up per version and at least three timed
encodes per version, alternating order between repetitions. Timings exclude
hashing, probing, full decode, metric passes and visual extraction. They include
lossless-reference decode, scaling, encoding and video-only MKV writes. Video
packet bytes are reported separately from container bytes. These are neither
whole-worker nor whole-film times.

Reference/output timestamps, frame counts, rate, dimensions, AV1 codec and
10-bit pixel format must match. Every output passes `-xerror` full decode.
Quality scores are reused across repetitions only when decoded-frame hashes
match. Cache identities include tool content, build manifests, reference
content, model, recipe, host/affinity and script hashes; replacing a tool at
the same path invalidates reuse. Inputs and outputs are never enqueued or
uploaded to production.

The runner retains exact command vectors, per-run logs, timings, build and
input hashes, per-frame metric files, CSV/JSON results and center-crop images
of matching source/baseline/candidate frames. The crops include the worst
frame-score change and a representative frame; they retain native pixel size.

## Reproduce

```bash
mage build:svtComparison
python3 scripts/av1_svt_versions.py \
  --baseline "$PWD/build/av1-lab/svt-4-3/tools/4.2.0/bin/ffmpeg" \
  --candidate "$PWD/build/av1-lab/svt-4-3/tools/4.3.0/bin/ffmpeg" \
  --metric-ffmpeg /absolute/path/to/fixed-metric-ffmpeg \
  --ffprobe /absolute/path/to/ffprobe \
  --manifest /absolute/path/to/scenes.json \
  --output "$PWD/build/av1-lab/svt-4-3/results" \
  --repeats 5 --crfs 25
```

The manifest is an array of `title` and absolute `reference` paths to decoded
lossless clips. Use a new output directory for a changed build or scene set.
For a quality ladder use `--crfs 24,25,26` in a separate directory; label
interpolated matches and never extrapolate. A matched mean alone does not
establish matched low-percentile or subjective quality.

Wait for all build/test work to finish before measuring speed. If another
workload is present, pass `--timing-contended`; the observed times are retained
but cannot validate the upstream performance claim. Do not change production
worker admission to obtain an idle host without an operator instruction.

## Compatibility validation on 2026-11-04

The performance measurements above are Linux/WSL2 measurements. Native Windows
validation below establishes compatibility; its encode speed was not compared.

- Fourteen Python lab tests pass, including stale-cache rejection, invalid
  frame/score rejection, temporal windows and guarded dependency replacement.
- Relevant Go tests pass for `worker/config`, `worker/step`, `worker/worker`,
  `worker/job` and `magefiles`, with race coverage for the worker paths.
  Native Windows disk round-trip tests preserve legacy implicit CRF25 and
  explicit qualities while new profiles persist CRF26.
- Both matched Linux toolchains pass FFmpeg/encoder-list inspection, runtime
  SVT version identification, 10-bit AV1 probing and strict decode. The candidate
  also completes a 24-frame 10-bit NVENC encode using the current p7/UHQ/CQ38
  profile on the RTX 5080 and passes strict decode.
- Native Windows FFmpeg/ffprobe 9.0.2 with SVT 4.3.0 built successfully.
  `TestWindowsBundle` passes in 0.84 seconds, including the runtime SVT version
  banner, preset 4 / CRF26, AV1 10-bit probing, strict decode, retained libaom,
  x265 and FDK AAC, mkvextract and private-runtime English OCR.
- The native Windows CPU worker E2E passes against an isolated Linux API and
  PostgreSQL, including the actual preset 4 / CRF26 recipe, AAC VBR5, subtitle
  OCR, duration/stream verification and upload (TestE2E: 10.74 seconds). The harness uses a unique
  control pipe so its controls cannot reach an existing worker.

Large/private evidence remains in the ignored worktree lab directory. Older
SVT 4.2.0 reports remain historical records.
