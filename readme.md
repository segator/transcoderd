# TranscoderD

![GitHub Release](https://img.shields.io/github/v/release/segator/transcoderd) ![CI](https://github.com/segator/transcoderd/actions/workflows/main.yml/badge.svg)

TranscoderD reduces video-library storage with distributed video encoding.
The worker defaults to AV1 on NVIDIA NVENC and also supports CPU SVT-AV1 and
the legacy software HEVC profile.

TranscoderD have a server and a worker application. The server will manage and schedule the jobs, the worker will do the actual encoding.
You can have as many workers you want, locally or remote in multiple machines.

## Features
- Encode AV1 with NVIDIA NVENC or CPU SVT-AV1; retain x265 HEVC as an explicit alternative
- Convert audio to AAC for better compatibility.
- Remove unwanted/duplicated audio tracks
- PGS (image like subtitles) to SRT: Plex and emby likes to burn this subtitle format to video, meaning almost guaranteed you are going to transcode the video when playing, this is why we convert it to SRT


## Prerequisites
- Docker
- Postgres database
- Grafana (Optional): For statistics and monitoring 


## Server Installation

### Config file transcoderd.yml

Replace it by your own values, remember to create manually the DB scheme and PG user.
```yaml
server:
    database:
      host: 192.168.1.55
      user: db_user
      password: db_pass
      scheme: encode    
    scheduler:
      sourcePath: /mnt/media
      deleteOnComplete: false # if true, the original file will be deleted after job is completed
      minFileSize: 100 # minimum file size to be considered for encoding
web:
  token: my_secret_token # replace it by your own secret
```

### Run
```bash
docker run -d \
       --name transcoder-daemon \
       --restart always \
       -p 8080:8080 \
       -v /mnt/media:/mnt/media 
      -v ./transcoderd.yml:/etc/transcoderd/config.yml \
      ghcr.io/segator/transcoderd:server-v2.5.8 # x-release-please-version
```

### Grafana Statistics
- Add postgres as datasource for grafana.
- Import this [grafana-dashboard](./grafana-dashboard.json) and change the data source to your own Postgres database.

### Completed output names

The server names completed outputs from the video codec detected in the uploaded
file, rather than from the worker's configured codec. AV1 outputs use an `av1`
label; H.264 uses `x264`; and HEVC/H.265 uses the library's conventional `x265`
label. Audio codec labels are normalized to `AAC`, and completed outputs use the
`.mkv` extension. If a destination already exists, the server preserves it and
tries `_encoded`, `_encoded-2`, and higher numbered suffixes until it reserves a
free name. The server verifies the output and saves its final path before it
records the job as complete.

`deleteOnComplete` only attempts to remove the source after the output has been
verified, the final path and size have been saved, and the completion event has
been stored. With `deleteOnComplete: false`, the original is preserved.


## Worker Installation

Follow the [worker deployment guide](docs/worker-deployment.md) for NVIDIA RTX
50-series drivers, Linux/Unraid/WSL GPU access, CPU-only workers, and separate
four-GPU/two-CPU job limits. The [encoding profile reference](docs/encoding-profiles.md)
explains every video/audio parameter and the [local validation record](docs/av1-worker-profile-validation.md)
contains the complete movie E2E result. The Docker image bundles FFmpeg and
codec libraries; the NVIDIA host driver and container runtime still need setup.

### Run

After creating the scratch directory and setting the server URL/token shell
variables from that guide, use a
published or locally built worker image containing the AV1 change (replace
`VERSION` with its tag). Older release images can have the legacy HEVC default.

```bash
docker run -d --name transcoderd-worker --restart unless-stopped \
  --network transcoderd --gpus all \
  --env TZ=Europe/Madrid \
  --mount type=bind,src=/srv/transcoderd/scratch,dst=/tmp \
  ghcr.io/segator/transcoderd:worker-VERSION --noUpdates \
  --web.domain "${TRANSCODER_SERVER_URL:?Set the server URL}" \
  --web.token "${TRANSCODER_TOKEN:?Set the server token}" \
  --worker.name gpu-5080 --worker.maxActiveJobs 4 \
  --worker.ffmpegConfig.videoCodec av1_nvenc \
  --schedule=22:00-10:00
```

For a CPU node, select `--worker.ffmpegConfig.videoCodec libsvtav1`, choose its
name and `--worker.maxActiveJobs` count, and omit the GPU device request. Encoder
settings use their codec defaults; encode concurrency inherits the active-job
limit. No per-worker YAML file is required. The shared schedule permits requests
daily from 22:00 until 10:00 in Europe/Madrid; accepted work finishes afterward.
The worker downloads media over the server API, so it needs a reachable server
URL and matching token rather than a direct media-library mount.

### Change job admission while the worker is running

Inspect or change how many jobs a running worker will request without restarting
its container:

```bash
docker exec transcoderd-worker /app/transcoderd-worker control status
docker exec transcoderd-worker /app/transcoderd-worker control set-max-jobs 1
docker exec transcoderd-worker /app/transcoderd-worker control set-max-jobs 0
docker exec transcoderd-worker /app/transcoderd-worker control set-max-jobs 2
```

The limit accepts non-negative integers. Zero stops requesting new jobs; a
positive value sets the maximum for future assignments. Existing and restored
jobs continue. If a request was already in flight when the limit changed, its
reservation appears in `status` and the server may still assign that job. Lowering
the limit below the number of active jobs does not interrupt them; the worker
waits until it has capacity before requesting more. The schedule remains an
additional restriction. The default is two, and `--worker.maxActiveJobs 0` starts
with new job requests disabled. Changes made through `control` last until the
worker restarts. This control changes job admission; it does not freeze FFmpeg.
It also does not resize the FFmpeg consumer pool: raising the limit above the
worker's startup maximum can admit more jobs into earlier/later pipeline stages,
but simultaneous encodes remain bounded by the pool created at startup. Restart
with a higher startup maximum (or explicit `worker.ffmpegConfig.parallelJobs`)
to increase encode concurrency.

## Development

The current behavior contracts are in [OpenSpec](./openspec/specs/). Start with the [baseline map and known gaps](./openspec/BASELINE.md), [configuration reference](./openspec/CONFIGURATION.md), and [HTTP protocol reference](./openspec/PROTOCOL.md) before changing a capability; contributor guidance is in [AGENTS.md](./AGENTS.md).

### Prerequisites
- [Go 1.26.10](https://go.dev/dl/)
- [Dagger CLI 0.20.6](https://docs.dagger.io/getting-started/install/)
- Mage CLI 1.17.2 (`go install github.com/magefile/mage@v1.17.2`)
- Docker with [buildx](https://docs.docker.com/build/install-buildx/) (for container builds)
- PostgreSQL (for integration tests, provided via testcontainers)

Mage is the local command interface; it delegates builds, tests, image publication, and release asset uploads to Dagger containers. CI pins Go, Mage, and Dagger directly and runs the same Mage targets.

### Build

```bash
mage build:all          # Build server and worker binaries
mage docker:all         # Build server and worker Docker images
mage docker:server      # Build only the server image
mage docker:worker      # Build only the worker image
```

### Test

```bash
mage test:unit          # Unit tests with coverage
mage test:race          # Unit tests with race detector
mage test:short         # Unit tests in short mode
mage test:integration   # Integration tests via Dagger services
mage test:av1lab        # AV1 benchmark utility unit tests in Dagger
mage test:all           # Unit + integration tests

# Run a single test
go test -v -run TestFunctionName ./path/to/package/...
```

### Lint

```bash
mage lint:check         # Run golangci-lint
mage lint:fix           # Run golangci-lint with auto-fix
```

### Docker

The versioned FFmpeg builder contains FFmpeg 9.0.2 with `libsvtav1`,
`libaom-av1`, and `av1_nvenc`; the PGS builder contains the OCR assets used by
the worker. App image builds reuse these validated builders. CI checks their
registry tags and builds/publishes any missing builder through Dagger. Rebuilding
the FFmpeg builder from source takes about 50 minutes; it is a separate target,
so ordinary app builds do not repeat that work. Builder compilation does not
require a GPU. GPU encoding at runtime requires a compatible NVIDIA GPU and
driver exposed to the worker container.
On Linux, configure NVIDIA Container Toolkit and run with `--gpus all`. The
worker image has also been tested on an RTX 5080 under WSL2 using direct device
and driver mounts. The worker defaults to the approved AV1 NVENC profile;
CPU SVT-AV1 and legacy x265 are explicit codec choices. See [AV1/NVENC
validation](docs/av1-nvenc-validation.md) and the [profile reference](docs/encoding-profiles.md).

```bash
mage docker:ffmpeg      # Build FFmpeg builder tarball locally
mage docker:pgs         # Build PGS builder tarball locally
mage publish:ffmpeg     # Build and push the FFmpeg builder
mage publish:pgs        # Build and push the PGS builder
mage publish:app        # Push server and worker images
```

### CI Pipeline

- **Lint**: runs `golangci-lint` on every push and PR
- **Build + Test + E2E**: runs Mage/Dagger tests, builds both Docker images, and runs Docker E2E on pushes and PRs
- **Docker Publish**: pushes server and worker images on `main`
- **Release**: [Release Please](https://github.com/googleapis/release-please) creates versioned releases on `main`; Dagger uploads the built release assets

### Project Structure

```
cmd/            # Shared CLI config (pflag, viper, mapstructure)
model/          # Domain types (Job, Event, Worker)
helper/         # Utilities (command exec, concurrent collections, progress)
server/         # Server application
  config/       # Server config structs
  repository/   # PostgreSQL repository (interface + SQL impl)
  scheduler/    # Job scheduling engine
  web/          # HTTP API (gorilla/mux)
worker/         # Worker application
  config/       # Worker config structs (FFmpeg, PGS, etc.)
  console/      # TUI rendering and logging
  ffmpeg/       # ffprobe wrapper
  job/          # Job execution context
  serverclient/ # HTTP client to server API
  step/         # Encoding pipeline steps
  worker/       # Worker coordination loop
integration/    # Integration tests (build tag: integration)
```
