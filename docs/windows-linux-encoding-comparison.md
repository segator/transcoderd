# Native Windows and WSL2 encoding comparison

Measured on 2026-10-26 on the same Ryzen 9 9900X3D (12 cores, 24 logical CPUs)
and RTX 5080, NVIDIA driver 621.01. Windows was Windows 11 Pro 10.0.26300;
Linux was the local WSL2 distribution, kernel 6.18.54.1-microsoft-standard-WSL2.
This is a movie-excerpt comparison, not a completed-film benchmark or a
bare-metal Linux comparison.

## Workload and isolation

The source is an SDR H.264 1920x1080, 25 fps movie excerpt with two stereo AC3
tracks. A stream-copy cut near the movie's ten-minute point retained a keyframe
preroll, producing a 126.2-second input; every encode used the same first
120-second output interval. Original timestamps were retained, yielding 2,999
displayed video frames and a 120.021-second output including AAC priming.

The Windows and Linux source files both have SHA-256
`be3ca444000acdbf92dfb37ccdfd62264679c7bafb74545eb448239b2b8f7cb6`.
Windows input/output was on native NTFS, and Linux input/output on WSL ext4.
No measured encode used a cross-filesystem source path. This follows
[Microsoft's filesystem guidance](https://learn.microsoft.com/en-us/windows/wsl/filesystems).

The existing CPU worker was stopped by the user. The GPU worker had admission
set to zero; the batch started only after its accepted jobs finished. Before
each run, the harness required `active=0` and `encodes=0` from that worker.
No production settings were changed by the benchmark. The GPU idle snapshot
reported 0% encoder utilization, 40.2 C and 23.8 W.

One five-second warmup per OS/encoder preceded three measured repetitions.
Runs were sequential with three-second gaps, and the second round reversed
the order. Times below are encoder-process wall times, excluding PowerShell
startup, source copying, worker setup, HTTP transfer, OCR and offline checking.
They include source decode, the normal width filter, video encoding, two
`libfdk_aac -vbr 5` audio encodes, and MKV muxing/writes.

## Versions and profiles

Both executables report FFmpeg 9.0.2, SVT-AV1 4.2.0 and the retained production
encoder dependencies. Linux used the FFmpeg copied from the current worker
image, run directly in WSL; Windows used the new portable bundle.

| Item | Linux/WSL2 | Native Windows |
| --- | --- | --- |
| Compiler | GCC 13.3.0 | GCC 14-posix / MinGW |
| Logical CPUs visible | 24 | 24 |
| Memory available to OS | WSL limit 20 GiB, no swap | Approximately 32 GiB |
| SVT assembly selected | AVX512 ICL | AVX512 ICL |
| SVT picture-control structures | 305 | 305 |

CPU settings were `libsvtav1`, preset 4, CRF25, `yuv420p10le`. GPU settings
were `av1_nvenc`, p7/UHQ, CQ38, VBR with no target bitrate, fullres multipass,
four middle B references, disabled split, QP 0-100 and `p010le` input surfaces.
Both used automatic FFmpeg threading and the production CPU width filter.
The complete argument arrays are retained with the raw results. No profile
defaults were changed to favor one OS.

## Measured times

| Encoder / OS | Three runs (s) | Median (s) | Video frames/s | MKV bytes |
| --- | --- | ---: | ---: | ---: |
| SVT CPU / WSL2 | 28.849, 30.182, 30.056 | 30.056 | 99.78 | 17,171,927 |
| SVT CPU / Windows | 33.871, 32.864, 33.452 | 33.452 | 89.65 | 17,171,927 |
| NVENC GPU / WSL2 | 22.175, 22.068, 22.104 | 22.104 | 135.68 | 16,500,963 |
| NVENC GPU / Windows | 21.489, 21.484, 21.453 | 21.484 | 139.59 | 16,500,963 |

Windows took **11.30% longer for CPU** and **2.80% less time for GPU**, using
the median WSL time as the denominator. These builds do not show a general
Windows speed advantage. The GPU difference is small; the CPU difference favors
the WSL build in all three repetitions.

FFmpeg's own internal `-benchmark` wall timers confirm the same pattern:
CPU medians 29.709s WSL / 33.090s Windows, and GPU medians 21.854s WSL /
21.226s Windows. The result is not explained by the outer timer including
PowerShell startup; that startup is excluded and process-launch overhead is
similar in both measurements.

This compares the distributed builds as used on this PC. Compiler/runtime,
scheduling, driver interface and filesystem differences are not isolated by
this test. It does not establish that virtualization caused the differences or
predict the result on bare-metal Linux, other clips or another machine.

## Output equivalence

Every measured output decoded without errors to 2,999 AV1 Main 10-bit 1080p
frames and retained both 48 kHz stereo AAC streams. For each encoder separately:

- All three repetitions on each OS have identical compressed packet payloads.
- Windows and WSL have identical SHA-256 fingerprints over the ordered video
  and audio packet sizes/data hashes, including packet counts.
- Decoding through the same Linux FFmpeg decoder produces identical hashes for
  every 10-bit video frame across the two OS outputs.
- Resolution, pixel format, stream count, languages and duration agree.

| Encoder | Video payload bytes | Decoded-frame-sequence SHA-256, both OS outputs |
| --- | ---: | --- |
| SVT CPU | 13,101,341 | `f6009b303f383ada5a77c07c55665ce3d33ac5a0c872b2d1c5382a171b1dee38` |
| NVENC GPU | 12,430,376 | `d857897308772462fd3b359650d775dedcf777aaac0ce76f134f91bc1afb49a9` |

The MKV file hashes differ, so whole-container byte identity is not claimed.
The compressed media payloads and decoded pictures are identical **within
each encoder's Windows/WSL pair**. CPU and GPU are different encoders at
different quality settings; their smaller/larger output sizes do not establish
a same-quality compression winner. A perceptual comparison between OS outputs
is unnecessary after exact decoded identity has been established.

## Reproduce and retained evidence

Run from WSL after preparing identical source copies and freeing the machine:

```bash
python3 scripts/compare_windows_linux.py \
  --linux-ffmpeg /lab/ffmpeg-linux \
  --windows-ffmpeg 'C:\TranscoderD\bin\ffmpeg.exe' \
  --source /lab/source.mkv --windows-source 'C:\lab\source.mkv' \
  --output /lab/batch --windows-output 'C:\lab\batch' \
  --duration 120 --repeats 3 --idle-container transcoderd-worker
```

The harness writes complete arguments and incremental timing JSON and refuses
to overwrite a completed/partial batch. The Windows timer uses a native
Stopwatch around the FFmpeg process. Linux uses a monotonic process wall timer.
Full raw evidence is in the ignored local `build/windows-linux-comparison/`:
source probes, executable hashes, environment snapshots, timing logs,
`batch1/comparison.json`, per-output probes and per-frame MD5 lists. Windows
outputs and native timer logs remain in the local Windows comparison folder,
and a copy is retained under `windows-artifacts/` beside the Linux results.
The retained `analyze.py` fingerprints all packets and decodes the representative
outputs; repeat payload identity extends those decoded checks to the repeats.

Executable SHA-256:

- Linux: `4e466d46ae62d1b28a16dac73440963d4b794efc194813f5bba00884c2a98d56`
- Windows: `86bed01240e4bd5c811404a6d701ee67dc06bf7770576f460a6fadd25c8b45ec`
