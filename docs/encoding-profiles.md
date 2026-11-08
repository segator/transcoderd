# Worker encoding profiles

The default video encoder is the RTX 5080 profile approved after watching a
complete film. It uses one fixed encode, followed by normal worker size and
duration verification. There is no mandatory VMAF prepass or content-dependent
retry. The [full movie E2E result](av1-worker-profile-validation.md) validates
the actual server, worker, subtitle, audio, and upload path. GPU setup and
separate CPU/GPU worker examples are in [deployment](worker-deployment.md).

## AV1 GPU: production default

```text
-threads 0
-pix_fmt p010le -c:v av1_nvenc -preset p7 -tune uhq
-rc vbr -cq 38 -b:v 0 -multipass fullres
-bf 4 -b_ref_mode middle -split_encode_mode disabled
-qmin 0 -qmax 100
```

| Parameter | Meaning and reason |
| --- | --- |
| `-threads 0` | Let FFmpeg decide applicable CPU threading. It does not set the number of GPU sessions. |
| `scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease` | CPU scaling filter used when either source dimension exceeds 2000 pixels or dimensions are unknown. The 80-pixel margin avoids resizing sources close to 1920; larger widths are reduced to 1920. This accounts for possible autorotation; a tall unrotated source might pass through without resizing. A 4K source is reduced; this is a separate size/quality change from AV1 compression. |
| 10-bit input to NVENC | Software decoding supplies `p010le`; hardware decoding supplies CUDA frames and requests internal conversion for 8-bit sources. AV1 remains 10-bit; conversion does not recover missing source detail. |
| `-c:v av1_nvenc` | Encode AV1 using NVIDIA's hardware video encoder. |
| `-preset p7` | Most quality-oriented NVENC preset. The user-approved film used it; it is not the fastest preset. Preset numbers cannot be compared to SVT preset numbers. |
| `-tune uhq` | Request ultra-high-quality tuning and its additional encoder analysis. More work can improve compression/quality; it is not a guarantee for every scene. |
| `-rc vbr` | Use variable bitrate rate control so scene complexity can influence allocation. |
| `-cq 38` | NVENC target quality: lower generally spends more bits. It is neither a VMAF score nor equivalent to SVT CRF38. This default is less conservative than CQ34 and can reduce size at some quality cost. |
| `-b:v 0` | No fixed average bitrate target in this CQ recipe. No `maxrate` ceiling is imposed on complex scenes. |
| `-multipass fullres` | NVENC performs its additional full-resolution analysis within the encode. It does not encode the entire film twice or run a VMAF pass. |
| `-bf 4` | Permit four B-frames between reference pictures. They use temporal prediction to improve coding efficiency. |
| `-b_ref_mode middle` | Allow the middle B-frame to serve as a reference. The tested hierarchical alternative did not justify replacing this recipe. |
| `-split_encode_mode disabled` | Do not divide a frame across NVENC engines. Multiple independent jobs can still use the GPU's encoding resources; the driver manages engines. |
| `-qmin 0 -qmax 100` | Enable paired minimum/maximum quantizer constraints. In the tested FFmpeg VBR implementation, supplying both is necessary to activate the maximum-QP constraint. The ceiling limits coarse quantization and helped the difficult LED-wall scene at a bitrate cost. It is not a hard VMAF floor. |

`videoCQ`, `videoQMax`, and `videoPreset` expose the named GPU overrides.
The remaining settings form the selected profile. AV1 uses the encoder's AV1
profile selection; the HEVC-specific `main10` option is not passed to AV1.
`videoCRF` and `videoProfile` are rejected with this encoder.

For eligible H.264 and HEVC 4:2:0 sources with dimensions at most 2000, the worker
uses NVDEC and feeds CUDA frames directly to NVENC. Eight-bit inputs use NVENC's
internal conversion to 10-bit output. It omits the explicit resize filter for
sources with both known dimensions at most 2000 pixels; FFmpeg may still insert a pixel
format converter when needed. Larger inputs retain software decoding
and the width filter shown above. Sources with missing probe metadata,
unsupported formats, invalid YUV color matrices or custom input arguments also
use software decoding. Recognizable hardware decoder initialization failures
before encoding starts retry once with software decoding. CPU encoders continue
to use software decoding. HDR preservation and 4K-to-1080p treatment need
their own deployment validation; the full-film pilot was SDR 1080p.

The [2026-11-08 pipeline experiment](av1-pipeline-results.md) separately measures
NVDEC under simultaneous SVT load, 8/10-bit output and retaining original 4K
dimensions, including the quality and size effect of the NVENC conversion.

## AV1 CPU: selectable SVT-AV1

The Linux builder and Windows bundle pin **SVT-AV1 4.3.0** behind FFmpeg 9.0.2.
The [version comparison](av1-svt-4-3-comparison.md) first isolated the library
change at CRF25. The selected **preset 4 / CRF26** profile then used 14.3–16.0%
fewer video bytes than SVT 4.2.0 / CRF25 on three 24-second excerpts, with mean
VMAF reductions of 0.286–0.363 point. The smaller files come with this measured
quality trade-off; these scenes do not establish whole-film quality.

```text
-threads 0
-pix_fmt yuv420p10le -c:v libsvtav1 -preset 4 -crf 26
```

`yuv420p10le` is planar 10-bit 4:2:0. `libsvtav1` performs AV1 encoding on CPU.
Its preset controls analysis effort: lower values are slower; larger values
are faster. CRF controls quality/bit allocation separately, with lower values
generally producing larger, higher-quality output. Changing the preset can
change quality at the same CRF, so compare at similar measured quality.

The time/size balanced default is preset **4 / CRF26**.
Presets **3, 5, and 6 are also selectable** through `videoPreset` (quoted
as a YAML string), and `videoCRF` is independently configurable. SVT manages
its own internal parallelism; automatic FFmpeg threading does not imply every
logical CPU stays saturated. Two simultaneous CPU jobs may improve aggregate
use but each competes for the same CPU and memory.

The [CPU preset comparison](av1-cpu-preset-comparison.md) records measured
trade-offs and balanced alternatives. A fixed CRF is not a promise that every
title meets the p5 VMAF target. CPU and GPU quality values are different scales;
equal numeric CRF/CQ settings are not a fair comparison.

The [SVT decoding comparison](svt-nvdec-decode-comparison.md) measured CPU and
NVDEC decoding both alone and alongside an NVENC job. On its two short source
excerpts NVDEC did not improve total time, so SVT continues to decode on CPU.

An earlier comparison used three representative 15-second clips and the
previous CRF25 CPU candidate. CQ38 produced 24.6–48.6% smaller video segments, with p5
VMAF changes of -0.13 to -2.37. SVT CRF25 produced 18.4–31.9% smaller
segments, with p5 changes of -0.48 to -1.37. These are short-scene measurements,
not a promise for all titles; difficult grain and motion can behave differently.

## Audio, subtitles, and container

The normal audio configuration remains **`libfdk_aac` with `audioVBR: 5`**.
The worker adds `-c:a:<index> libfdk_aac -vbr 5` for each selected audio track.
FDK's VBR mode 5 is its highest VBR quality mode; the final bitrate depends on
content and channel layout, as described in the
[FFmpeg codec reference](https://ffmpeg.org/ffmpeg-codecs.html#libfdk_005faac).
We retain the existing tested audio policy. Both the short E2E fixture and the
full-film test now exercise the normal FDK default. No listening
comparison between AAC encoders was performed, so video VMAF results do not
establish an audio quality ranking.

The worker selects one audio stream per language, preferring channel count
and then bitrate; it does not automatically preserve every commentary or
alternate track. Stream maps and language/title metadata follow that existing
selection. Image subtitles go through extraction and OCR before FFmpeg maps
the converted SRT inputs. The output is MKV. Chapters are disabled with
`-map_chapters -1`.

Timestamp and mux options retain the existing worker behavior:

| Option | Purpose |
| --- | --- |
| `-fflags +genpts` | Generate missing presentation timestamps where applicable. |
| `-analyzeduration 2147483647 -probesize 2147483647` | Allow extensive source probing to identify streams; these are analysis limits, not an encoder quality setting. |
| `-copyts -avoid_negative_ts make_zero` | Retain input timing while shifting negative timestamps for muxing. |
| `-max_muxing_queue_size 9999 -max_interleave_delta 0` | Preserve the worker's buffering/interleaving policy. |
| `-flags +global_header` | Request global codec headers where used. |
| `-nostdin -nostats -progress pipe:1 -hide_banner` | Run unattended and provide machine-readable progress to the worker. |
| `-y` | Write the output in the isolated job workspace without an interactive overwrite prompt. |

Normal verification measures the completed MKV and rejects growth over the
source. When container durations differ beyond the configured tolerance, the
worker compares the first video stream's packet timeline to distinguish video
truncation from longer subtitle or audio tracks. It does not calculate
perceptual quality. The [legacy HEVC reference](legacy-hevc-encoding.md) preserves x265
medium/CRF21/main10 for rollback and later benchmark comparisons.
