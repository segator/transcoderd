# AV1 library conversion lab

## Decision to make

Measure whether AV1 is worth using for a film and series library currently made
mostly of H.264 and H.265. Compare 10-bit AV1 produced by the RTX 5080's NVENC
with software SVT-AV1 at **matched measured quality**. The preferred result
balances storage saved and time spent encoding. No production encoder default
changes until the results include representative library material.

The reproducible runner is [`scripts/av1_lab.py`](../scripts/av1_lab.py). It reads
source files and writes references, AV1 encodes, logs, and metrics under
`build/av1-lab/`. Put private clips in `benchmark-input/`; both directories
are ignored by Git. The lab never edits a source file. Do not commit inventory,
results, VMAF JSON, encoded media, or file paths from a private library.

## Sample design

Inventory the library before choosing samples. Include at least three titles
from each populous source class, with films and episodes represented separately:

| Dimension | Requested coverage |
| --- | --- |
| Source codec | H.264 and H.265, including both 8-bit and 10-bit material |
| Resolution | 720p, 1080p, and 4K where present |
| Content | Dark scenes, grain/noise, fast action, low-motion dialogue, animation, clean CGI, fine textures |
| Duration | Short episode, long episode, and feature film |
| Color | SDR first; HDR/PQ/HLG handled in a separate color-managed study |
| Existing compression | High-bitrate and already tightly compressed sources |

Use full original files when possible. The runner samples 60-second windows at
approximately 15%, 50%, and 85% of each title's usable timeline. Add complete
source clips for known hard scenes, opening credits, and fades as separate
inputs. A title shorter than the window uses its full duration. Every candidate
decodes the same source window; a lossless FFV1 copy of those decoded frames is
used for VMAF. The runner checks that candidate and reference frame counts
match. Longer 3–10 minute windows should confirm steady
throughput and thermal behavior after the initial sweep. Run at least one
complete episode or film with the final candidate to detect scene and muxing
problems missed by windows.

The default input folder is intentionally empty; the owner will add their own
files. Public pilot samples are [Tears of Steel](https://download.blender.org/demo/movies/)
and [Caminandes](https://download.blender.org/demo/movies/). They exercise the
method, but cannot establish savings for the private H.265 library. A short
[Xiph sample](https://media.xiph.org/video/derf/) can exercise high frame rate;
its VP9 source is outside the H.264/H.265 projection population.

## Candidate grid and controls

The runner tests NVENC `p5/hq` and `p7/uhq`, each at CQ 18/22/26/30/36/42, against
SVT-AV1 presets 4/6 at CRF 10/16/22/28/36/44. These are starting points, not a
codec-independent quality scale. Extend a range when a desired VMAF target
falls outside tested scores. NVENC uses VBR with zero target bitrate, 10-bit
`p010le`, full-resolution multipass, four B-frames, middle B-frame references,
and split-frame encoding disabled. SVT uses 10-bit `yuv420p10le`. Timed jobs
include CPU decoding of the H.264/H.265 source and AV1 encoding, without audio
or subtitle processing. Record the actual FFmpeg build, NVIDIA driver, CPU,
GPU, and power/thermal state with the result.

For each candidate, record video packet bytes, all-frame mean VMAF, fifth
percentile and minimum frame VMAF, frame count, and wall-clock encode time.
Run three repeats on the longer windows and use the median time. Inspect VMAF
JSON and the output video at the worst-scoring scenes, especially texture,
grain, dark gradients, subtitles burned into video, and movement. Averages do
not rule out an objectionable local defect. Check playback compatibility on
the intended clients before converting the library.

The default [VMAF model](https://github.com/Netflix/vmaf/blob/master/resource/doc/faq.md)
scores decoded source frames against decoded AV1 frames, not against the
pre-compression camera master. Re-encoding cannot restore detail lost in the
source. The runner skips HDR because an SDR VMAF score is unsuitable for
comparing PQ/HLG content. Treat 4K SDR default-model scores as relative signals
and inspect the native-resolution result; calibrate quality targets with human
review before applying them to the library. No single VMAF threshold proves
perceptual transparency.

## Run the lab

Use the FFmpeg binary from the AV1 worker image (or an equivalent build with
`av1_nvenc`, `libsvtav1`, FFV1 and `libvmaf`). On this RTX 5080 WSL2 host the
tested binary is `build/ffmpeg9-av1`; see
[`av1-nvenc-validation.md`](av1-nvenc-validation.md) for GPU access and the
validated image. `ffprobe` must be available too.

```bash
mkdir -p benchmark-input build/av1-lab
python3 scripts/av1_lab.py inventory --input benchmark-input \
  --packet-bytes --output build/av1-lab/library.jsonl
python3 scripts/av1_lab.py run --input benchmark-input \
  --ffmpeg build/ffmpeg9-av1 --ffprobe ffprobe \
  --output build/av1-lab/library-runs
python3 scripts/av1_lab.py report \
  --results build/av1-lab/library-runs/results.jsonl \
  --inventory build/av1-lab/library.jsonl \
  --target-vmaf 95 --max-gpu-premium 15 \
  --output build/av1-lab/matched.csv
```

The `run` command resumes completed points from `results.jsonl`. It deletes
large encoded videos and FFV1 references by default, while retaining logs,
per-frame VMAF JSON, and measurements. Use `--keep-encodes` and
`--keep-references` for visual review. A reference window can consume several
gigabytes, especially at 4K; run a small sample first and ensure enough space.
`--window`, `--windows`, `--repeats`, `--profiles`, `--nvenc-qualities`,
`--svt-qualities`, `--threads`, and `--max-files` narrow or extend the sweep.
Do not compare times from runs made with different CPU thread limits or with
other heavy workloads competing for the GPU.

For a quick public smoke run after downloading a sample under `build/`:

```bash
python3 scripts/av1_lab.py run --input build/tears-of-steel_teaser.mp4 \
  --ffmpeg build/ffmpeg9-av1 --output build/av1-lab/smoke \
  --window 8 --windows 1 --repeats 1 \
  --profiles nvenc-p7,svt-p4 \
  --nvenc-qualities 20,30,36,42 --svt-qualities 10,28,44
python3 scripts/av1_lab.py report \
  --results build/av1-lab/smoke/results.jsonl \
  --target-vmaf 95 --output build/av1-lab/smoke/matched.csv
```

## Comparison and library forecast

For each window and profile, interpolate video bytes and encode time **within**
the measured rate/quality curve at a chosen VMAF target. Never extrapolate
outside the tested range. Compare NVENC and SVT at that common target, including
the fifth-percentile score and visual review. `report` finds the smallest SVT
video at matched quality, then the fastest NVENC candidate whose video is no
more than the configured storage premium (15% by default) above it. Vary that
premium (for example 5%, 10%, 15%, 20%) and the VMAF target to expose the
size/time frontier. Interpolation is a screening estimate: rerun CQ/CRF values
inside promising brackets until actual encodes are within roughly 0.5 VMAF of
the target before making a final comparison. Prefer the NVENC choice only if
its speed gain is worth its observed extra bytes for the actual library. Some
windows may favor NVENC in both size and time; others may favor SVT
compression. Report coverage, median and spread across titles, not only a
best-case clip.

`inventory --packet-bytes` sums actual source video packets; this is slower
than metadata probing but works for files with no declared video bitrate.
The library estimate groups files by source codec, resolution, bit depth, and SDR/HDR.
Within represented classes it projects new video bytes using the measured
window output-to-source-video rate and adds the original non-video bytes.
It projects serial encoding hours from measured seconds per source second.
Unrepresented classes remain uncovered and are counted separately. Container
overhead, audio/subtitle handling, chapter preservation, video rate changes
within a title, and concurrency can change the real total. The estimate is a
screening tool; validate it with full-title conversions. Already efficient H.265
sources may show little gain after a quality-preserving transcode and can be
left untouched.

All NVENC runs keep `-split_encode_mode disabled`; split-frame modes are not a
test variable. Measure encode duration for each NVENC and SVT-AV1 run
sequentially under the same conditions. This answers whether NVENC's quality
and size trade-off is worthwhile for the library without adding multi-session
throughput as a separate experiment. If source formats and color conversion
allow it, a GPU decode/scale pipeline can be measured separately from CPU
decode; record end-to-end elapsed time rather than encoder fps alone.

## Public pilot on the RTX 5080

On 2026-10-12, FFmpeg 9.0.2 with driver 616.92 completed the runner on two
H.264 Blender clips and a short 60 fps VP9 clip. Every smoke encode decoded
and scored the same number of frames as its reference. The table below is an
**interpolated screening estimate**, using one eight-second window and one
timing run per candidate at mean VMAF 95:

| H.264 sample | NVENC p7 AV1 video / time | SVT preset 4 AV1 video / time |
| --- | ---: | ---: |
| Tears of Steel teaser | 1.91 MB / 1.93 s | 2.91 MB / 2.56 s |
| Caminandes animation | 2.64 MB / 2.21 s | 3.06 MB / 3.66 s |

At these two points NVENC was faster and produced fewer bytes, but the coarse
quality brackets and short windows cannot establish the winner for films or
series. The `report` command correctly withheld a library forecast: these are
only two H.264 titles, and there is no H.265 source sample yet. Longer sources
and the private library are needed for a sustained estimate.

## Acceptance criteria

1. At least three representative titles per major source class and difficult
   content type, including H.264 and H.265, with any uncovered library bytes
   called out.
2. Every compared run has the same input frames, full-frame VMAF, measured
   video bytes, at least three steady-run timing repeats, and manual review of
   poor-scoring scenes.
3. NVENC and SVT choices are confirmed by measured encodes near the same
   quality target, with a sensitivity table for storage premium and target quality.
4. Storage and serial encode-hour forecasts include a range across titles;
   full-title validation refines the time estimate before a library-wide
   decision.
5. Final recommendation states which classes to convert, which to skip, the
   selected settings, expected storage change, elapsed time, and playback
   compatibility findings.
