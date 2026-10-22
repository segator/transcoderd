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

In the worker Docker template, mount the scratch directory and supply the CLI
worker arguments shown below as the container command,
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
  --env TZ=Europe/Madrid \
  --device /dev/dxg \
  --mount type=bind,src=/usr/lib/wsl/lib,dst=/usr/lib/wsl/lib,readonly \
  --mount type=bind,src=/usr/lib/wsl/drivers,dst=/usr/lib/wsl/drivers,readonly \
  --env LD_LIBRARY_PATH=/usr/lib/wsl/lib \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/tmp \
  ghcr.io/segator/transcoderd:worker-VERSION --noUpdates \
  --web.domain "${TRANSCODER_SERVER_URL:?Set the server URL}" \
  --web.token "${TRANSCODER_TOKEN:?Set the server token}" \
  --worker.name gpu-5080 --worker.maxActiveJobs 4 \
  --worker.ffmpegConfig.videoCodec av1_nvenc \
  --schedule=22:00-10:00
```

These WSL paths are host-specific and are not a Linux/Unraid configuration.
No privileged container was needed. The opt-in GPU E2E helper uses these same
mounts. See [GPU validation](av1-nvenc-validation.md).

## CLI-only CPU and GPU workers

Run one worker instance per encoder. The ordinary node-specific settings are
its name, `maxActiveJobs`, and `videoCodec`. Encoder defaults supply preset,
quality, pixel format, audio, and mux options. Omitted encode concurrency
inherits `maxActiveJobs`; the advanced `ffmpegConfig.parallelJobs` override is
optional. Shared connection and schedule arguments can be copied between nodes.
No per-worker YAML file or configuration mount is required. Mount scratch at
`/tmp` so the default worker workspace persists across container restarts.

Choose a built/published image containing this change. Set `TRANSCODER_IMAGE`
to its `ghcr.io/segator/transcoderd:worker-VERSION` tag,
`TRANSCODER_SERVER_URL` to the reachable server URL, and `TRANSCODER_TOKEN`
to your server credential in your shell. Do not put a real token in a committed
script. Create `/srv/transcoderd/scratch` on each node before starting.

GPU example on Linux with the NVIDIA Container Toolkit:

```bash
docker run -d --name transcoderd-gpu --restart unless-stopped --gpus all \
  --env TZ=Europe/Madrid \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/tmp \
  "${TRANSCODER_IMAGE:?Set the worker image tag}" --noUpdates \
  --web.domain "${TRANSCODER_SERVER_URL:?Set the server URL}" \
  --web.token "${TRANSCODER_TOKEN:?Set the server token}" \
  --worker.name gpu-5080 --worker.maxActiveJobs 4 \
  --worker.ffmpegConfig.videoCodec av1_nvenc \
  --schedule=22:00-10:00
```

CPU example:

```bash
docker run -d --name transcoderd-cpu --restart unless-stopped \
  --env TZ=Europe/Madrid \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/tmp \
  "${TRANSCODER_IMAGE:?Set the worker image tag}" --noUpdates \
  --web.domain "${TRANSCODER_SERVER_URL:?Set the server URL}" \
  --web.token "${TRANSCODER_TOKEN:?Set the server token}" \
  --worker.name cpu-9900x3d --worker.maxActiveJobs 2 \
  --worker.ffmpegConfig.videoCodec libsvtav1 \
  --schedule=22:00-10:00
```

Both examples accept new jobs every day from 22:00 inclusive until 10:00
exclusive in Europe/Madrid. This includes weekends. Accepted jobs finish after
the cutoff. See [scheduling](worker-scheduling.md) for timezone and overrun rules.

Use distinct names on a shared scratch directory. Workers download sources
from the server API and need no direct library mount. Provision scratch for all
accepted source copies and outputs. For a local Docker server, connect both
containers to its user-defined network and use the server's DNS alias; a remote
server needs a URL reachable from each container.

The GPU profile is the approved p7/UHQ/CQ34 recipe; CPU selection resolves its
software profile automatically. Advanced preset/CRF overrides remain available
for experiments and rollback. The [encoder reference](encoding-profiles.md)
describes the exact current defaults. Whole-film follow-up measurements will
determine the recommended general CPU preset; the prior short-scene projections
are not completed movie results.

`maxActiveJobs` includes download, subtitles, encode, verification, and upload.
With no encode override, four accepted GPU jobs can encode together and two
accepted CPU jobs can encode together. The server assigns available jobs; it
does not route particular titles or source codecs to CPU versus GPU. Increasing
slots does not multiply throughput linearly. Mixed four-GPU-plus-two-CPU load
and wall-plug energy have not been measured.

## What the build pipeline does

Mage and Dagger build the application images from a Debian runtime and two
versioned builder images. `builder-ffmpeg-9.0.2-av1` contains FFmpeg 9.0.2 with
`libsvtav1`, `libaom-av1`, `av1_nvenc`, x265, and `libfdk_aac`. The separate PGS
builder supplies .NET/OCR binaries and language data. The server and worker
images copy the needed files from those builders. An NVIDIA GPU is not needed
to compile these images.

`.github/workflows/main.yml` runs on pushes and pull requests targeting `main`,
plus manual dispatch. It logs into GHCR, runs Go tests and the AV1 benchmark
lab, checks the versioned FFmpeg and PGS builder manifests, builds and publishes
a missing builder when needed, builds server/worker images, and runs the Docker
E2E test. Publishing application/release images is restricted to non-PR runs.
The builder step needs package push access when a builder is absent; fork runs
without that access need an available published builder or a maintainer build.

The ordinary CI E2E run selects **SVT-AV1 CPU**, so a hosted runner needs no
GPU. CI validates compilation and the CPU pipeline; it cannot validate NVENC
hardware without a GPU runner. The local opt-in RTX 5080 tests cover that
hardware path. A stacked PR whose base is not `main` does not trigger this
main-only build workflow; the local results are recorded separately.

Local build commands are:

```bash
mise install                    # Go, Mage, and Dagger from .mise.toml
mise exec -- mage build:all     # Go server and worker binaries
mise exec -- mage docker:all    # Server and worker images via Dagger
mise exec -- mage test:e2e      # Short CPU AV1 server/worker pipeline check
```

`mage docker:ffmpeg` and `mage docker:pgs` build local builder tarballs;
`mage publish:ffmpeg` and `mage publish:pgs` build and push versioned builders
after verification. Changing FFmpeg or codec dependencies requires a new
builder tag in `magefiles/magefile.go`, then validation and publishing of that
tag. CI reuses existing builder tags and builds missing ones; it does not
rebuild a present tag just because source changed. See the documented
[legacy HEVC profile](legacy-hevc-encoding.md) for rollback and benchmark
reference.

If overriding `PROJECT_VERSION` for a local build, use a semantic version such
as `0.0.0-av1-test`; the updater parses the version even with `--noUpdates`.
Arbitrary image/branch labels are not valid application version metadata.
