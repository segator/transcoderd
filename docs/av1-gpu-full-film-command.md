# AV1 GPU full-film pilot command

The FFmpeg argument vector used for the completed feature-film pilot is saved
in [`av1-gpu-full-film-command.json`](av1-gpu-full-film-command.json). It
includes argument order, stream maps, audio and subtitle settings, timestamp
and mux settings. Machine-specific paths are replaced with placeholders so the
repository does not expose local media-library paths. The file records the
command that ran, not a TranscoderD configuration or a universal AV1 default.

The video configuration the user approved after watching the complete output
was:

```text
-filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"
-pix_fmt p010le -c:v av1_nvenc -preset p7 -tune uhq
-rc vbr -cq 34 -b:v 0 -multipass fullres
-bf 4 -b_ref_mode middle -split_encode_mode disabled
-qmin 0 -qmax 100
```

The two source audio streams were encoded separately with `libfdk_aac -vbr 5`.
The source PGS subtitle was extracted and converted to SRT before this FFmpeg
command, then mapped from a second input. FFmpeg did not perform the OCR. The
full argument vector also records `-threads 0`, input analysis settings, stream
metadata, and MKV mux options; these details should be preserved when
comparing another end-to-end encode with this result.

## Reproduce or adapt

The JSON is an array of strings, so it can be passed directly to a process
without shell quoting changes:

```python
import json
import subprocess

with open("docs/av1-gpu-full-film-command.json", encoding="utf-8") as f:
    command = json.load(f)
subprocess.run(command, check=True)
```

Replace the executable, input, subtitle, and output placeholders before
running it. The command has no `-y` flag; FFmpeg will ask before overwriting an
existing output. The source and output are both MKV files.

## Result and environment

| Item | Recorded value |
| --- | --- |
| Source | H.264, 1920×1080 SDR, 89 min 15 s, 21,022,090,356 bytes |
| Completed output | AV1 + two AAC tracks + SRT, 1,492,848,730 bytes |
| Encode wall time | 915.71 s (15 min 16 s), FFmpeg stage only |
| Output SHA-256 | `d3ee70f58a441f52e8b7cd2d4e53b927903084889c16c3c73ed0a6e119cfd714` |
| Encoder | Project FFmpeg 9.0.2 build, NVIDIA GeForce RTX 5080 |

The user watched this completed encode and approved its visual quality. Six
sampled one-minute windows were also scored against the source, with the
weakest fifth-percentile VMAF at 91.168. These samples do not establish a
whole-film or whole-library quality guarantee. The encoded file is at
`build/av1-lab/plex/full-film-pilot/bruce/bruce-av1-gpu-full.mkv` in the local
workspace. The original is left intact.

## NVIDIA Video Codec SDK 13.1 review

The current Mage/Dagger FFmpeg builder compiles
[FFmpeg 9.0.2](https://ffmpeg.org/download.html) on a CUDA 13.3.1 development
image using `ffmpeg-build-script` tag `v9.0.10` (see
`magefiles/magefile.go`).
That script [pins `nv-codec-headers` 13.1.15.0](https://github.com/markus-perl/ffmpeg-build-script/blob/v9.0.10/src/10-versions.sh),
corresponding to the latest released Video Codec SDK 13.1
[headers](https://github.com/FFmpeg/nv-codec-headers/releases). The worker
image and local FFmpeg binary report version 9.0.2. The local binary exposes
`av1_nvenc` options for `uhq`, `fullres`
multipass, `middle` and `hierarchical` B-frame references, lookahead level,
temporal filtering, and split-encode mode. NVIDIA driver 617.42 on the test
host exceeds the [610 minimum](https://github.com/FFmpeg/nv-codec-headers/blob/master/README)
listed for those headers. CUDA toolkit version,
SDK header version, driver version, and FFmpeg feature integration are distinct.
The builder image tag and FFmpeg binary do not themselves prove that every SDK
13.1 API is used by FFmpeg.

NVIDIA's [SDK 13.1 overview](https://developer.nvidia.com/blog/nvidia-video-codec-sdk-13-1-zero-copy-transcode-av1-b-frames-and-frame-accurate-seek/)
identifies four features relevant to this workflow:

- **Hierarchical AV1 B-frames:** FFmpeg exposes the option, but NVIDIA's
  [encoder guide](https://docs.nvidia.com/video-technologies/video-codec-sdk/13.1/nvenc-video-encoder-api-prog-guide/index.html#b-frames-as-reference)
  requires multipass disabled and lookahead disabled or at level zero. A local
  earlier 15-B-frame trial with those restrictions scored p5 VMAF 79.507 on a
  difficult LED-wall minute and 89.341 on an easier concert minute. A later
  [matched CQ34/UHQ comparison](av1-hierarchical-bframe-followup.md) found
  mixed scene results and worse size at matched quality on the weak film
  minute. It is not a suitable drop-in replacement for the approved recipe.
- **UHQ plus iterative encoding:** UHQ is already used. SDK 13.1 additionally
  permits applications to re-encode a frame at multiple settings while using
  UHQ's lookahead and temporal filtering. The installed `av1_nvenc` option
  list has no iterative-encode switch, and the current command does not use
  that API. This would need separate encoder integration and a measured reason
  to pay the extra encode cost.
- **NVDEC-to-NVENC `CUarray` zero-copy:** NVIDIA demonstrates this in an SDK
  sample for throughput and memory efficiency. This command uses FFmpeg's CPU
  width filter and handles audio and subtitles; it does not use that sample's
  shared-surface pipeline. Zero-copy would not directly change AV1 bitrate at
  fixed encoder settings. Its main potential value here is throughput across
  sessions, which has not been validated for this workflow.
- **Decoder statistics and frame-accurate seek:** Potentially useful for a
  future sampling or analysis tool. They do not improve a sequential full-film
  encode merely by upgrading headers.

For now, retain the approved command unchanged. Any future SDK feature should
be tested against its file size, weak-scene visual quality, VMAF, encode time,
and energy on the same clips before changing the worker's profile.
