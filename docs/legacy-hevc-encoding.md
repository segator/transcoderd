# Legacy HEVC worker profile

Before the AV1 worker change, TranscoderD's default video profile was software
HEVC: `libx265`, `medium`, CRF `21`, 10-bit `main10`. It remains selectable with
`worker.ffmpegConfig.videoCodec: libx265` as a rollback and benchmark reference.
The large-source pilot used FFmpeg 8.1 from the former `worker-v2` image for its
HEVC baseline; the current worker image contains FFmpeg 9.0.2. Encoder version
and all other settings should be recorded when comparing new measurements.

The historical worker generated this command shape (stream numbers and paths
vary by title):

```text
ffmpeg -fflags +genpts -nostats -progress pipe:1 -hide_banner \
  -threads 0 -analyzeduration 2147483647 -probesize 2147483647 \
  -i <downloaded-source.mkv> [-i <converted-subtitle.srt> ...] \
  -max_muxing_queue_size 9999 \
  -map 0:<video-stream> -avoid_negative_ts make_zero -copyts \
  -map_chapters -1 -flags +global_header \
  -filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease" \
  -pix_fmt yuv420p10le -c:v libx265 -crf 21 -profile:v main10 -preset medium \
  [-map 0:<audio-stream> -metadata:s:a:<index> "title=<language> (<layout>)" \
    -c:a:<index> libfdk_aac -vbr 5 ...] \
  [subtitle maps, dispositions and metadata] -y <output-encoded.mkv>
```

The old generator placed `-y` after the output path and assembled one command
string before parsing it back into arguments. The AV1 worker uses a structured
argument vector and places `-y` before the output path; these changes do not
alter the intended HEVC video settings. For a video-only codec comparison,
hold the input frames, scaling, pixel format, audio/subtitle treatment, and
quality target constant. CRF 21 and NVENC CQ 34 are different scales; equal
numbers or default profiles do not imply equal visual quality.

The old profile can be restored on a worker with:

```yaml
worker:
  ffmpegConfig:
    videoCodec: libx265
    videoPreset: medium
    videoCRF: 21
    videoProfile: main10
```

Omit AV1-only `videoCQ` and `videoQMax` when selecting HEVC. The benchmark
source and scene methodology are recorded in the
[AV1 benchmark lab](av1-benchmark-lab.md); its measured HEVC baseline used
this profile on the then-current worker image.

For the pilot's **video-only 60-second scenes**, the reproducible command was
the following. Replace the source, output directory, and seek time for each
scene. The 8.1 image version is intentional when comparing against those
historical measurements:

```bash
docker run --rm --network none --user "$(id -u):$(id -g)" \
  --volume /path/to/source.mkv:/input.mkv:ro \
  --volume /path/to/output:/out \
  --entrypoint /usr/bin/ffmpeg ghcr.io/segator/transcoderd:worker-v2 \
  -fflags +genpts -nostats -hide_banner -threads 0 \
  -analyzeduration 2147483647 -probesize 2147483647 \
  -ss 600 -i /input.mkv -t 60 \
  -map 0:v:0 -an -sn -dn -fps_mode passthrough \
  -filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease" \
  -pix_fmt yuv420p10le -c:v libx265 -crf 21 \
  -profile:v main10 -preset medium -y /out/hevc-current-q21.mkv
```

This scene command deliberately excludes audio and subtitles. Compare its
video packet bytes to AV1 scene outputs, or run complete TranscoderD jobs for
whole-file size and pipeline-time comparisons.
