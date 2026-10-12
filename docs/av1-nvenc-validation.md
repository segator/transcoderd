# AV1/NVENC validation on an RTX 5080

Validated on 2026-10-12 with the worker image from this change, an NVIDIA
GeForce RTX 5080, driver 616.92, WSL2, and FFmpeg 9.0.2. The image includes
`av1_nvenc`, `libsvtav1`, `libaom-av1`, and `libvmaf`. FFmpeg inside the worker
container produced decodable 10-bit AV1 with `av1_nvenc`. It also completed a
separate H.264 GPU decode, `scale_cuda`, and AV1 NVENC encode test. These tests
exercise the bundled FFmpeg; TranscoderD's job pipeline still defaults to HEVC
and does not yet have validated AV1/NVENC job settings.

## Container GPU access

On a Linux Docker host, configure the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
and pass `--gpus all` to the worker container. The local WSL2 Docker daemon
rejected `--gpus all` because it had no NVIDIA runtime configured. For this
test, passing `/dev/dxg` and the WSL driver files directly worked without a
privileged container. This WSL2 command reproduces the synthetic 10-bit encode
after `make buildcontainer-worker` on the current branch:

```bash
WORKER_IMAGE="ghcr.io/segator/transcoderd:worker-$(git branch --show-current | sed 's/[^a-zA-Z0-9._-]/-/g')"
docker run --rm --device=/dev/dxg \
  --mount type=bind,src=/usr/lib/wsl/lib,dst=/usr/lib/wsl/lib,readonly \
  --mount type=bind,src=/usr/lib/wsl/drivers,dst=/usr/lib/wsl/drivers,readonly \
  --env LD_LIBRARY_PATH=/usr/lib/wsl/lib \
  --entrypoint /usr/bin/ffmpeg "$WORKER_IMAGE" \
  -hide_banner -f lavfi -i testsrc2=size=1280x720:rate=30 \
  -frames:v 60 -pix_fmt p010le -c:v av1_nvenc \
  -preset p7 -tune uhq -rc vbr -cq 20 -b:v 0 \
  -multipass fullres -bf 4 -b_ref_mode middle \
  -split_encode_mode disabled -f null -
```

The output reported AV1 Main, 10-bit `p010le`, and 60 encoded frames. A second
run wrote an MKV file and decoded it without errors. To exercise GPU decode and
scaling too, use `-hwaccel cuda -hwaccel_output_format cuda` before `-i`, and
`-vf scale_cuda=w=1280:h=-2:format=p010le` before `-c:v av1_nvenc`. That path
also completed in the worker container on the RTX 5080.

The RTX 5080 has [two ninth-generation NVENC engines](https://www.nvidia.com/en-us/geforce/graphics-cards/50-series/rtx-5080/).
The tested quality-oriented command disables split-frame encoding because
[NVIDIA documents a quality loss from splitting a frame between engines](https://docs.nvidia.com/video-technologies/video-codec-sdk/13.1/nvenc-video-encoder-api-prog-guide/index.html#multi-nvenc-split-frame-encoding-in-hevc-and-av1).
Two independent encode sessions can still use the card's aggregate encode
capacity. NVIDIA's [FFmpeg guide](https://docs.nvidia.com/video-technologies/video-codec-sdk/13.1/ffmpeg-with-nvidia-gpu/index.html)
describes `p7`, UHQ tuning, full-resolution multipass, and VBR constant-quality
options. Lower `-cq` values request higher quality and larger files; `20` is a
test point, not an established optimum.

## Provisional quality and size comparison

Source: an eight-second segment (10–18 seconds) of Blender's
[Tears of Steel teaser](https://download.blender.org/demo/movies/tears-of-steel_teaser.mp4.zip),
1920×800 at 24 fps. The high-bitrate H.264 source was decoded to a lossless
FFV1 reference. All outputs contain video only and use 10-bit 4:2:0. VMAF was
measured against the *decoded H.264 source*, not the original camera footage.
Results for this one mixed live-action/CG segment:

| Encoder settings | Video file size | VMAF |
| --- | ---: | ---: |
| NVENC AV1 `p7`, `uhq`, full-resolution multipass, CQ 20, split disabled | 7.164 MB | 97.760 |
| NVENC AV1 `p5`, `hq`, full-resolution multipass, CQ 21, split disabled | 7.155 MB | 98.349 |
| SVT-AV1 preset 4, CRF 10 | 6.251 MB | 97.797 |

The two NVENC points have nearly equal size, yet `p5/hq` scored higher on this
sample. SVT-AV1 used about 13% less space than `p7/uhq` at nearly equal VMAF.
The CUDA/NVENC encode completed at about 4.9× real time for this short segment;
startup, input decoding, and the short duration limit the usefulness of that
throughput number. These observations do not establish a universal preset or
GPU-versus-CPU winner. They need more varied footage, matched-rate comparisons,
visual inspection, power measurements, and longer runs before setting production
defaults. Converting an 8-bit source to 10-bit does not recover source detail.

To reproduce the comparison, extract the reference and run the encoders and
VMAF filter with the same 192 frames:

```bash
ffmpeg -ss 10 -i tears-of-steel_teaser.mp4 -t 8 -an \
  -c:v ffv1 -level 3 reference.mkv
ffmpeg -i reference.mkv -an -pix_fmt p010le -c:v av1_nvenc \
  -preset p7 -tune uhq -rc vbr -cq 20 -b:v 0 \
  -multipass fullres -bf 4 -b_ref_mode middle \
  -split_encode_mode disabled nvenc.mkv
ffmpeg -i reference.mkv -an -pix_fmt yuv420p10le -c:v libsvtav1 \
  -preset 4 -crf 10 svt.mkv
ffmpeg -i nvenc.mkv -i reference.mkv \
  -lavfi libvmaf=log_fmt=json:log_path=nvenc-vmaf.json -f null -
```

Run the NVENC command inside a GPU-enabled worker container, as above, to test
the distributed image. The comparison files and source media are local test
artifacts and are not committed to the repository.
