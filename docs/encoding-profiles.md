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
-filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"
-pix_fmt p010le -c:v av1_nvenc -preset p7 -tune uhq
-rc vbr -cq 34 -b:v 0 -multipass fullres
-bf 4 -b_ref_mode middle -split_encode_mode disabled
-qmin 0 -qmax 100
```

| Parameter | Meaning and reason |
| --- | --- |
| `-threads 0` | Let FFmpeg decide applicable CPU threading; software decode and scaling still use CPU. It does not set the number of GPU sessions. |
| `scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease` | Existing CPU scaling filter: cap width at 1920 without enlarging narrower sources, preserving aspect ratio. A 4K source is reduced; this is a separate size/quality change from AV1 compression. |
| `-pix_fmt p010le` | Supply 10-bit 4:2:0 surfaces to NVENC. AV1 is encoded as 10-bit; converting 8-bit input does not recover missing source detail. |
| `-c:v av1_nvenc` | Encode AV1 using NVIDIA's hardware video encoder. |
| `-preset p7` | Most quality-oriented NVENC preset. The user-approved film used it; it is not the fastest preset. Preset numbers cannot be compared to SVT preset numbers. |
| `-tune uhq` | Request ultra-high-quality tuning and its additional encoder analysis. More work can improve compression/quality; it is not a guarantee for every scene. |
| `-rc vbr` | Use variable bitrate rate control so scene complexity can influence allocation. |
| `-cq 34` | NVENC target quality: lower generally spends more bits. It is neither a VMAF score nor equivalent to SVT CRF34. |
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

There is no `-hwaccel cuda` or `scale_cuda` in this recipe. Decode and the width
filter remain on CPU; encode runs on GPU. Earlier 1080p trials found effectively
the same throughput with GPU decode/scale, while actual resizing changed the
image and needed separate quality validation. This is the tested reference
path. HDR preservation and 4K-to-1080p treatment need their own deployment
validation; the full-film pilot was SDR 1080p.

## AV1 CPU: selectable SVT-AV1

```text
-threads 0
-filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"
-pix_fmt yuv420p10le -c:v libsvtav1 -preset 4 -crf 23
```

`yuv420p10le` is planar 10-bit 4:2:0. `libsvtav1` performs AV1 encoding on CPU.
Its preset controls analysis effort: lower values are slower; larger values
are faster. CRF controls quality/bit allocation separately, with lower values
generally producing larger, higher-quality output. Changing the preset can
change quality at the same CRF, so compare at similar measured quality.

The provisional time/size balanced fallback is preset **4 / CRF23**.
Presets **3, 5, and 6 are also selectable** through `videoPreset` (quoted
as a YAML string), and `videoCRF` is independently configurable. SVT manages
its own internal parallelism; automatic FFmpeg threading does not imply every
logical CPU stays saturated. Two simultaneous CPU jobs may improve aggregate
use but each competes for the same CPU and memory.

The [CPU preset comparison](av1-cpu-preset-comparison.md) records measured
trade-offs and balanced alternatives. A fixed CRF is not a promise that every
title meets the p5 VMAF target. CPU and GPU quality values are different scales;
equal numeric CRF/CQ settings are not a fair comparison.

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
source or excessive duration mismatch. It does not calculate perceptual
quality. The [legacy HEVC reference](legacy-hevc-encoding.md) preserves x265
medium/CRF21/main10 for rollback and later benchmark comparisons.
