# AV1 hierarchical B-frame follow-up on the RTX 5080

## Question and method

We had already tested AV1 B-frames: the approved full-film command uses four
B-frames and `-b_ref_mode middle`, and a seven-B-frame middle-reference trial
lowered quality on the weak *Bruce Lee* minute. We had also tried SDK 13.1's
**hierarchical** reference mode on a concert, but that older smoke test used
HQ, CQ30, no paired QP cap, and no full-resolution multipass. Its result was
not a matched comparison with the approved CQ34/UHQ recipe.

This follow-up encoded three existing 60-second, lossless FFV1 references with
the same FFmpeg 9.0.2 build and RTX 5080. The reference is the decoded source,
not the camera master. All variants used 10-bit P010, AV1 NVENC p7, UHQ, VBR
CQ34, `-b:v 0`, `-qmin 0 -qmax 100`, and disabled split encoding. The approved
variant used `-multipass fullres -bf 4 -b_ref_mode middle`. The hierarchical
variant used `-multipass disabled -bf 15 -b_ref_mode hierarchical`
`-rc-lookahead 0 -lookahead_level 0`, as required by NVIDIA's
[SDK 13.1 encoder guide](https://docs.nvidia.com/video-technologies/video-codec-sdk/13.1/nvenc-video-encoder-api-prog-guide/index.html#b-frames-as-reference).
The remaining encoder arguments were identical. Therefore this tests the
**hierarchical mode and its required multipass/lookahead changes together**;
it cannot attribute a difference to the reference structure alone.

Each VMAF score compared all frames of the one-minute output to its matched
reference, with timestamps aligned by frame index. The table gives video
packet bytes only; audio, subtitles, mux overhead, full-film GOP context, and
worker transfer time are excluded. Encode seconds are local FFmpeg wall times.
These are scene results, not a whole-library projection.

| One-minute scene | Recipe | Video MB | p5 VMAF | Lowest frame | Encode s |
| --- | --- | ---: | ---: | ---: | ---: |
| *Bruce Lee*, weakest sampled minute | Approved middle/4 | 6.544 | 91.304 | 84.478 | 11.23 |
| *Bruce Lee*, weakest sampled minute | Hierarchical/15, CQ34 | 4.155 | 88.300 | 84.336 | 6.82 |
| Concert, LED-wall minute | Approved middle/4 | 124.203 | 91.966 | 83.219 | 14.31 |
| Concert, LED-wall minute | Hierarchical/15, CQ34 | 120.292 | 92.460 | 80.379 | 8.58 |
| Concert, difficult peak | Approved middle/4 | 134.690 | 83.565 | 63.017 | 13.71 |
| Concert, difficult peak | Hierarchical/15, CQ34 | 127.082 | 83.532 | 62.753 | 8.57 |

Hierarchical/15 was roughly 38–40% quicker on these minutes. At the same CQ,
it saved 36.5% of video bytes on the weak film minute but lost **3.00 p5 VMAF
points**, below the agreed 91–92 target. On the LED wall it saved 3.1% and
improved p5 by 0.49 points, although its lowest frame scored 2.84 points
worse. On the concert peak it saved 5.6% with essentially unchanged p5, but
both recipes were far below the target on that scene. Results are strongly
content dependent.

## Quality-matched check on the weak film minute

The CQ number is not a quality score. To compare compression at similar
measured quality, we lowered hierarchical mode's CQ while keeping its other
arguments fixed:

| Hierarchical setting | Video MB | p5 VMAF | Encode s |
| --- | ---: | ---: | ---: |
| 15 B-frames, CQ34 | 4.155 | 88.300 | 6.82 |
| 15 B-frames, CQ30 | 4.415 | 89.035 | 6.94 |
| 15 B-frames, CQ28 | 4.604 | 89.312 | 6.71 |
| 15 B-frames, CQ34, qmax96 | 4.466 | 88.510 | 6.96 |
| 15 B-frames, CQ34, qmax92 | 4.640 | 88.730 | 6.97 |
| **15 B-frames, CQ16** | **8.935** | **91.323** | **7.00** |
| 7 B-frames, CQ34 | 4.258 | 88.423 | 6.14 |
| **Approved middle/4, CQ34** | **6.544** | **91.304** | **11.23** |

The quality-matched hierarchical setting found in this small sweep, CQ16,
produced **36.5% more video bytes**. It encoded faster, but
did not improve size at the same measured quality. Seven hierarchical
B-frames at CQ34 did not repair the quality gap. Tightening the QP ceiling
from 100 to 96 or 92 increased size modestly but left p5 below 89, so it did
not offer an efficient fix in this limited sweep.

## Decision

Keep the approved full-film command for the selected AV1 pilot. Hierarchical
B-frames are available and faster here, and they may be useful on particular
concert scenes, but these tests do not support replacing the fixed library
profile. The weak film minute alone loses the target quality at the same CQ;
restoring it used more space. The concert peak still fails the quality target
under both modes. No hierarchical full-film encode was run, because the
targeted test did not show a quality-matched storage gain on the film.

The exact commands, logs, outputs, timing JSON, and per-frame VMAF JSON are
under `build/av1-lab/plex/large-source-benchmark/hierarchical-approved-recipe/`.
The local reproducer is
`build/av1-lab/plex/run_hierarchical_approved_recipe.py`.
