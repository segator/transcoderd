# AV1 decode, scaling and bit-depth experiment

Measured on 2026-11-08 on the Ryzen 9 9900X3D (24 logical CPUs) and RTX 5080, NVIDIA driver 621.01. FFmpeg 9.0.2 and SVT-AV1 4.3.0 were used throughout. Production encodes were drained before the timed experiment. Production settings were not changed during this experiment; see the current encoding profile for subsequent changes.

A production worker started a separate encode after the final timed batch completed. Subsequent offline validation and quality analysis are excluded from all reported performance timings.

The operator prioritizes storage and throughput, accepting a small visible difference in difficult scenes when worthwhile. Original resolution is preferred. This report separates that output-policy choice from acceleration at fixed dimensions.

CPU uses SVT preset 4 / CRF26; GPU uses NVENC p7/UHQ, CQ38, full-resolution multipass, four B frames with middle references, disabled split and QP0–100. Depth screening also includes CPU CRF24/28 and GPU CQ36/40.

## Native Windows: identical fixed batches

The installed Windows bundle supports NVDEC/NVENC and NVENC internal 8-to-10-bit conversion (`-highbitdepth 1`), but does not contain `scale_cuda`. Native confirmation therefore uses direct NVDEC-to-NVENC, preserving the source dimensions.

Each CPU and GPU job encodes exactly 2400 frames from the same H.264 source. One or two CPU jobs run with two GPU jobs. Three alternating repetitions are measured for each route. All jobs start in one batch; the clock stops only when every job finishes. GPU MKVs are retained, frame-counted and strictly decoded. All CPU output progress is checked too.

| CPU + GPU jobs | GPU preprocessing | Median batch seconds | Time reduction vs current | GPU-process CPU seconds |
|---|---|---:|---:|---:|
| 1 + 2 | cpu-scale | 22.432 | +0.00% | 58.141 |
| 1 + 2 | cpu-native | 22.327 | +0.47% | 57.766 |
| 1 + 2 | nvdec-direct | 20.661 | +7.89% | 10.203 |
| 2 + 2 | cpu-scale | 37.893 | +0.00% | 58.562 |
| 2 + 2 | cpu-native | 37.529 | +0.96% | 57.172 |
| 2 + 2 | nvdec-direct | 34.649 | +8.56% | 10.422 |

GPU-process CPU seconds are summed scheduled CPU time, not elapsed time or energy. Native CPU/GPU fps in the private report divide fixed frame counts by whole-batch time. The WSL overlap-fps experiment is a separate sustained-load screen: faster GPU jobs can overlap a different part of the looping CPU excerpt, so its apparent CPU-fps change is not a matched-work speedup.

## 8 versus 10 bits at the current numeric quality

Six real SDR excerpts cover three H.264 8-bit sources, two HEVC 10-bit 1080 sources and one HEVC 10-bit 4K source. Every compared pair has the same decoded input frames and native resolution. There is no assumed universal 5% cost for 10-bit output.

| Scene | Encoder | 8-bit video bytes vs 10-bit | Mean VMAF delta (8 − 10) | p5 delta |
|---|---|---:|---:|---:|
| film-8bit-a | cpu | +1.07% | -0.225 | -0.169 |
| film-8bit-b | cpu | -1.09% | -0.336 | -0.500 |
| film-8bit-c | cpu | -1.52% | -0.163 | -0.061 |
| hevc10-sdr-a | cpu | +5.15% | -0.260 | -0.329 |
| hevc10-sdr-b | cpu | -1.67% | -0.157 | -0.484 |
| hevc10-sdr-4k | cpu | +1.69% | -0.263 | -0.168 |
| film-8bit-a | gpu | +7.48% | -0.113 | -0.516 |
| film-8bit-b | gpu | +6.92% | -0.308 | -0.371 |
| film-8bit-c | gpu | +4.11% | -0.156 | -0.119 |
| hevc10-sdr-a | gpu | +18.68% | -0.310 | -0.217 |
| hevc10-sdr-b | gpu | +3.06% | -0.111 | -0.322 |
| hevc10-sdr-4k | gpu | +15.23% | -0.174 | -0.179 |

At CQ38, 8-bit NVENC is larger and scores lower in every measured scene. CPU results have small mixed size changes and consistently lower scores. This does not support dropping depth as a storage optimization for these profiles. Equal CRF/CQ values alone do not establish equal measured quality.

### Actual CPU encodes near the same mean VMAF

These are measured encodes, not interpolated curves. The 10-bit reference is CRF26. The listed 8-bit point is the measured point nearest its mean VMAF; p5 is shown separately because equal means do not imply equal difficult-scene quality.

| Scene | 8-bit CRF | Mean VMAF delta | p5 delta | 8-bit byte change |
|---|---:|---:|---:|---:|
| film-8bit-a | 25 | +0.069 | +0.414 | +17.95% |
| film-8bit-b | 25 | -0.076 | -0.073 | +9.08% |
| film-8bit-c | 25 | +0.033 | +0.318 | +6.73% |
| hevc10-sdr-4k | 24 | +0.066 | +0.269 | +28.20% |
| hevc10-sdr-a | 24 | +0.021 | +0.046 | +22.81% |
| hevc10-sdr-b | 24 | +0.021 | +0.212 | +12.32% |

## Decode-path quality and integrity

CPU and NVDEC decoded-frame hashes match in all eight tested excerpts, including HEVC 8/10-bit, SDR/HDR and 4K. Under WSL, NVDEC with P010 format conversion produces identical AV1 packet payloads to software decode in all six SDR quality excerpts.

Direct NVDEC of an 8-bit source with NVENC internal conversion is a distinct candidate: it changes the encoded result slightly. The following comparison scores every frame of the retained 2400-frame native Windows outputs from the two-CPU/two-GPU batch.

| Native route | Video bytes | Mean VMAF | p5 VMAF | SSIM |
|---|---:|---:|---:|---:|
| cpu-scale | 22391148 | 92.3327 | 86.8107 | 0.950824 |
| nvdec-direct | 22556887 | 92.3226 | 86.7102 | 0.950855 |

Direct-path change: **+0.74% video bytes**, mean VMAF **-0.0101**, p5 **-0.1005**. This is measured on one 100-second window, not a whole-film guarantee.

Omitting the resolution filter on already-small sources preserved AV1 packet payloads in **8/10** checked CPU/GPU cases. Merely retaining a matching-size filter does not prove it is doing expensive geometric resizing; pixel-format conversion can still be necessary.

The other two cases are CPU encodes of HEVC 10-bit sources: decoded pixel hashes still match, but the existing scale filter clears the chroma sample position to unspecified while omission retains the source value `left`. The AV1 sequence header and MIME codec string therefore differ. The baseline records this discrepancy and its subsequent resolution; it is not silently treated as bitstream equality.

Both independent native GPU sessions produced identical AV1 packets in all **18/18** batches. Their first 200 packets match the corresponding Linux quality reference in **18/18** cases; prefix comparison deliberately avoids different end-of-window lookahead.

## Original 4K versus the existing width cap

Removing the cap preserves four times as many pixels in these sources. This changes the output and must not be presented as a same-quality acceleration result. On the HEVC 10-bit 4K timing excerpt, CPU median time changed from 6.361 to 19.634 seconds; GPU changed from 4.787 seconds with the current capped recipe to 11.516 seconds with native 4K and NVDEC.

The following 240-frame outputs measure actual video and MKV bytes. Source-window bytes sum compressed packets whose presentation timestamps match the same displayed-frame window. This is a short-window screen; whole-file admission must still use completed MKV size, including selected audio/subtitles.

| Scene | Encoder | Output dimensions | Video bytes | MKV bytes | Video bytes vs source window |
|---|---|---|---:|---:|---:|
| hevc10-sdr-4k | cpu | 1920×1080 | 1784754 | 1786997 | -89.78% |
| hevc10-sdr-4k | cpu | 3840×2160 | 5521070 | 5523358 | -68.38% |
| hevc10-sdr-4k | gpu | 1920×1080 | 1559935 | 1562184 | -91.06% |
| hevc10-sdr-4k | gpu | 3840×2160 | 4696001 | 4698265 | -73.10% |
| hevc8-sdr-4k | cpu | 1920×800 | 1127913 | 1130147 | -2.98% |
| hevc8-sdr-4k | cpu | 3840×1600 | 2435056 | 2437304 | +109.45% |
| hevc8-sdr-4k | gpu | 1920×800 | 945925 | 948139 | -18.64% |
| hevc8-sdr-4k | gpu | 3840×1600 | 2011789 | 2014045 | +73.05% |

## Scope and recommendation

- Keep 10-bit CPU/GPU output for this candidate; measured compression does not support an automatic downgrade to 8 bits. Quality settings are a separate storage/quality control.
- NVDEC is a useful candidate for supported source formats, especially alongside CPU encoding. The installed Windows path uses direct NVDEC input and NVENC internal conversion where necessary; it cannot use the absent CUDA scaling filter.
- Preserve original dimensions as requested, while explicitly accepting the measured additional 4K time/bytes. Retain whole-file size verification; efficient HEVC sources may be poor transcode candidates.
- HDR encodes were checked for functionality, not perceptual acceptance. An SDR VMAF score cannot approve 10-to-8-bit HDR conversion.
- These are direct FFmpeg video experiments, not full worker jobs. Audio/OCR/subtitles, downloads/uploads and complete-title quality remain outside this timing study. Confirm a selected profile on complete titles and through the worker before changing the observable production contract.

All quality scoring uses explicit 10-bit inputs, with every frame scored by VMAF and SSIM. Native 4K SDR uses the 4K model; absolute scores from different models are not a shared quality scale. Native-size center crops include worst measured frame deltas; still-frame review cannot establish temporal or whole-film perceptual equivalence.

Inspection of selected SDR crops, including the worst measured frame delta in the native 100-second comparison, showed no obvious additional blocking or ringing from direct NVDEC. Both routes suppress some original grain. The RGB24 still previews do not approve HDR or 10-bit gradient rendering on the intended display.

Reproduction: [suite and methodology](av1-pipeline-suite.md). Anonymized measured points: [quality](benchmarks/av1-pipeline-quality.csv), [isolated timing repetitions](benchmarks/av1-pipeline-timings.csv), [native Windows batches](benchmarks/av1-pipeline-windows.csv). Private media, source paths, exact command vectors, resource traces, per-frame scores, decoded hashes and visual artifacts remain under the ignored local lab directory.
