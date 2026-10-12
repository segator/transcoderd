# Design: AV1-ready FFmpeg image

## Builder and runtime

Use `nvidia/cuda:13.3.1-devel-ubuntu24.04` only for the FFmpeg compilation stage. The pinned `markus-perl/ffmpeg-build-script` tag `v9.0.10` builds FFmpeg 9.0.2, SVT-AV1, libaom, x265, FDK AAC, and NVIDIA codec headers. Its NVENC path requires `nvcc`; the CUDA development image supplies it. Export only the resulting FFmpeg executables to a small `scratch` builder image. The application runtime remains Debian Trixie and receives those executables, as before.

Set CUDA compute capability 75 for a reproducible build on hosts without GPU access. This controls compiled CUDA kernels; NVENC itself uses NVIDIA's video encoding driver interface. A compatible NVIDIA GPU, driver, and container GPU passthrough are still required to run `av1_nvenc`. Expose NVIDIA's `video` driver capability in the runtime image.

Limit parallel build jobs to four by default to fit CI memory. Local builders may override `FFMPEG_BUILD_JOBS`. The builder stage fails if `libsvtav1`, `libaom-av1`, `av1_nvenc`, `libx265`, or `libfdk_aac` is missing. The runtime stage checks that the copied FFmpeg exposes AV1 encoders.

## Distribution and rollout

Tag the new artifact `builder-ffmpeg-9.0.2-av1`. The Makefile supplies the tag to application builds; the Dockerfile has the same default for direct builds. CI checks for that tag and builds it once if absent. The previous `builder-ffmpeg` tag remains available to earlier revisions. Local builds can load the new builder image and build the worker against it before the tag is pushed.

## Scope and verification

This change only packages encoder support. `worker.ffmpegConfig.videoCodec` still defaults to `libx265`, and the command still passes HEVC-oriented profile, preset, CRF, and pixel format settings. AV1 job settings and CPU/GPU performance comparisons require a separate change.

Build the FFmpeg builder and worker images locally. Check the FFmpeg version and encoder list inside the worker image, run a short SVT-AV1 encode, and run relevant Go tests and strict OpenSpec validation. An NVENC encode requires a compatible GPU passed into Docker and may be unavailable on the local Docker daemon.
