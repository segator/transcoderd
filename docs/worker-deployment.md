# CPU and NVIDIA worker deployment

The worker image includes the application, FFmpeg 9.0.2, SVT-AV1, AV1 NVENC,
x265, `libfdk_aac`, ffprobe, mkvextract, .NET, PgsToSrt, Tesseract, and OCR
language data. Pulling a built image requires no Go compiler, FFmpeg package,
CUDA Toolkit, or Video Codec SDK installation on the worker host.

The **GPU driver and GPU access remain host responsibilities**. A container
cannot install the host kernel driver or make an unavailable GPU accessible.
The default video profile requires an NVIDIA GPU that supports its options;
the complete job has been validated on an RTX 5080. CPU-only hosts explicitly
select `libsvtav1`. See [the encoder reference](encoding-profiles.md) for settings
and [the local validation](av1-worker-profile-validation.md) for measured results.

## NVIDIA GeForce RTX 50-series on Linux

1. Install a distribution-supported NVIDIA driver that recognizes the GPU.
   Follow NVIDIA's [open GPU kernel-module installation guide](https://docs.nvidia.com/datacenter/tesla/driver-installation-guide/latest/kernel-modules.html)
   for the current Linux driver packaging, including Blackwell systems.
   This image's `nv-codec-headers` 13.1.15.0 requires **driver 610.0 or newer**
   on Linux and Windows, according to the
   [pinned header README](https://github.com/FFmpeg/nv-codec-headers/blob/n13.1.15.0/README).
   The RTX 5080 test used 617.42. A driver that merely recognizes a 50-series
   GPU can still be too old for this FFmpeg build's codec API.
2. Install the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
   on the Docker host, following its distribution-specific instructions.
   Configure Docker with `sudo nvidia-ctk runtime configure --runtime=docker`
   and restart Docker during an appropriate maintenance window.
3. Expose the GPU to the worker with `--gpus all`, or choose a specific GPU.
   The image already declares `NVIDIA_VISIBLE_DEVICES=all` and
   `NVIDIA_DRIVER_CAPABILITIES=compute,utility,video`. The `video` capability
   is needed for the driver-side encoding libraries.

First confirm that `nvidia-smi` lists the expected card and driver on the host.
Then check the application image, substituting a built/published image tag:

```bash
docker run --rm --gpus all --entrypoint nvidia-smi \
  ghcr.io/segator/transcoderd:worker-VERSION

docker run --rm --entrypoint ffmpeg \
  ghcr.io/segator/transcoderd:worker-VERSION -hide_banner -encoders

docker run --rm --gpus all --entrypoint ffmpeg \
  ghcr.io/segator/transcoderd:worker-VERSION \
  -hide_banner -f lavfi -i testsrc2=size=1920x1080:rate=24 -t 3 \
  -pix_fmt p010le -c:v av1_nvenc -preset p7 -tune uhq \
  -rc vbr -cq 34 -b:v 0 -multipass fullres \
  -bf 4 -b_ref_mode middle -split_encode_mode disabled \
  -qmin 0 -qmax 100 -f null -
```

Listing `av1_nvenc` proves it was compiled in; the last command actually
initializes the GPU encoder. A missing CUDA/encode library usually points to
container GPU exposure or driver capabilities. An unsupported codec API points
to an old driver. Installing another FFmpeg inside the running container is
not the fix. The worker preserves the container's driver library search path
when launching its FFmpeg child.

## Unraid

Use Unraid's supported NVIDIA driver mechanism, normally the
[Nvidia-Driver plugin](https://forums.unraid.net/topic/98978-plugin-nvidia-driver/),
and select a driver meeting the image's 610 minimum for the installed GPU.
Follow the plugin's reboot/runtime instructions for the installed Unraid
version. The GPU must be available to the host driver rather than reserved
exclusively for a VM.

In the worker Docker template, mount the configuration and scratch directory,
select the NVIDIA runtime/GPU using the plugin's instructions, and expose
`NVIDIA_VISIBLE_DEVICES` (the GPU UUID or `all`) and
`NVIDIA_DRIVER_CAPABILITIES=compute,utility,video`. Some installations use
`--runtime=nvidia`; Docker hosts with GPU device requests use `--gpus all`.
Verify with the real encoder smoke command above. Do not apply Ubuntu `apt`
driver instructions to Unraid or install a second driver inside the image.
If the installed Unraid/plugin combination cannot supply a compatible driver,
use a CPU worker or a different GPU host until it can.

## Windows with WSL2

Install a compatible NVIDIA **Windows** driver and keep WSL updated.
[NVIDIA's WSL guide](https://docs.nvidia.com/cuda/wsl-user-guide/index.html)
explains that Windows supplies WSL's GPU driver; do not install a Linux GPU
display driver inside WSL. Docker Desktop's GPU path or a configured NVIDIA
runtime may provide normal GPU device requests.

The local validation used a Docker daemon inside WSL with no NVIDIA runtime.
Its working alternative exposes the WSL GPU interface and driver mounts:

```bash
docker run --rm --name transcoderd-gpu --network transcoderd \
  --device /dev/dxg \
  --mount type=bind,src=/usr/lib/wsl/lib,dst=/usr/lib/wsl/lib,readonly \
  --mount type=bind,src=/usr/lib/wsl/drivers,dst=/usr/lib/wsl/drivers,readonly \
  --env LD_LIBRARY_PATH=/usr/lib/wsl/lib \
  --mount type=bind,src=/srv/transcoderd/gpu.yml,dst=/etc/transcoderd/config.yml,readonly \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/scratch \
  ghcr.io/segator/transcoderd:worker-VERSION --noUpdates
```

These WSL paths are host-specific and are not a Linux/Unraid configuration.
No privileged container was needed. The opt-in GPU E2E helper uses these same
mounts. See [GPU validation](av1-nvenc-validation.md).

## Four GPU jobs and two CPU jobs on one host

Run two instances with distinct worker names. Each instance selects one video
encoder and has an independent job limit. Use the same server URL and token
in both files, and ensure that the server is reachable on the Docker network.
The token below is a placeholder to replace with your own server credential.

The examples use a user-defined network named `transcoderd`. Create it once
and attach the existing server from the README with the DNS alias used below:

```bash
docker network create transcoderd
docker network connect --alias transcoderd-server transcoderd transcoder-daemon
```

For a server on another host, set `web.domain` to its reachable URL instead.
An existing deployment can use its own network and server DNS name.

```yaml
# /srv/transcoderd/gpu.yml
web:
  domain: http://transcoderd-server:8080
  token: replace-this-token
worker:
  name: gpu-5080
  temporalPath: /scratch
  maxActiveJobs: 4
  ffmpegConfig:
    videoCodec: av1_nvenc
    videoPreset: p7
    videoCQ: 34
    videoQMax: 100
    parallelJobs: 4
    threads: 0
    audioCodec: libfdk_aac
    audioVBR: 5
```

```yaml
# /srv/transcoderd/cpu.yml
web:
  domain: http://transcoderd-server:8080
  token: replace-this-token
worker:
  name: cpu-9900x3d
  temporalPath: /scratch
  maxActiveJobs: 2
  ffmpegConfig:
    videoCodec: libsvtav1
    videoPreset: "4"
    videoCRF: 23
    parallelJobs: 2
    threads: 0
    audioCodec: libfdk_aac
    audioVBR: 5
```

Create `/srv/transcoderd/scratch` and the two configuration files first. On a
normal Linux host with the NVIDIA Container Toolkit configured:

```bash
docker run -d --name transcoderd-gpu --restart unless-stopped \
  --network transcoderd --gpus all \
  --mount type=bind,src=/srv/transcoderd/gpu.yml,dst=/etc/transcoderd/config.yml,readonly \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/scratch \
  ghcr.io/segator/transcoderd:worker-VERSION --noUpdates

docker run -d --name transcoderd-cpu --restart unless-stopped \
  --network transcoderd \
  --mount type=bind,src=/srv/transcoderd/cpu.yml,dst=/etc/transcoderd/config.yml,readonly \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/scratch \
  ghcr.io/segator/transcoderd:worker-VERSION --noUpdates
```

The CPU example explicitly selects the balanced preset-4 override. Omitting
`videoPreset` still selects preset 3, the compression-oriented CPU default.
CRF23 is a configurable starting value, not a guarantee of p5 VMAF on every
title. See the [matched-quality CPU comparison](av1-cpu-preset-comparison.md).

The different names produce different scratch subdirectories. Workers download
sources from the server API; they do not need the library mounted directly.
Provision scratch space for all accepted source copies and output files.
`maxActiveJobs` includes download, subtitles, encode, verification, and upload;
`parallelJobs` limits simultaneous FFmpeg encodes. With these values, at most
four GPU and two CPU jobs can be active. The server assigns available jobs;
it does not route particular source codecs or titles to CPU versus GPU.

Slot counts do not multiply speed linearly. A 120-second 1080p trial measured
1.85× aggregate GPU throughput at four jobs versus one, and only 1.17× CPU
throughput at two jobs versus one. The mixed four-GPU-plus-two-CPU workload
has not been measured and can contend for CPU, memory, and I/O. For the
balanced library migration, begin with the GPU worker and enable CPU capacity
only when its extra storage saving or host utilization justifies it.

## What the build pipeline does

`Dockerfile` compiles FFmpeg with a CUDA development image and pinned
`ffmpeg-build-script` tag, validates the required encoders, and exports the
binaries as `builder-ffmpeg-9.0.2-av1`. It separately builds the PGS/.NET/OCR
assets. Runtime worker images copy these assets and the compiled Go worker.
An NVIDIA GPU is not needed to compile these images.

`.github/workflows/main.yml` runs on pushes and PRs to `main`, plus manual
dispatch. It logs into GHCR, runs Go tests, checks the versioned FFmpeg and PGS
builder manifests, builds/publishes a missing builder, builds server/worker
images, and runs the Docker E2E test. Publishing application/release images
is restricted to non-PR runs. The builder step requires package push access
when a builder is absent; fork runs without that access need an available
published builder or a maintainer build.

The ordinary CI E2E run selects **SVT-AV1 CPU**, so a hosted runner needs no
GPU. CI validates compilation and the CPU pipeline; it cannot validate NVENC
hardware without a GPU runner. The local opt-in RTX 5080 tests cover that
hardware path. A stacked PR whose base is not `main` does not trigger this
main-only build workflow; the local results are recorded separately.

Local build commands are:

```bash
make build-builder-ffmpeg       # Expensive; needed when the builder changes
devbox run -- make build        # Go binaries and server/worker images
devbox run -- make test-e2e     # CPU Docker pipeline check
```

`make publish-builder-ffmpeg` builds and pushes the versioned builder after
verification. Changing FFmpeg or codec dependencies requires a new builder
tag in both the Dockerfile and Makefile, then validation and publishing of
that tag. CI reuses an existing tag; it does not rebuild it merely because
the Dockerfile source changed. The current code remains usable with the
documented [legacy HEVC profile](legacy-hevc-encoding.md).

If overriding `PROJECT_VERSION` for a local build, use a semantic version such
as `0.0.0-av1-test`; the updater parses the version even with `--noUpdates`.
Arbitrary image/branch labels are not valid application version metadata. The
Makefile also creates a separate branch-named image tag automatically.
