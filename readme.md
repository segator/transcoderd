# TranscoderD

![GitHub Release](https://img.shields.io/github/v/release/segator/transcoderd) ![CI](https://github.com/segator/transcoderd/actions/workflows/main.yml/badge.svg)

TranscoderD is a solution that helps to reduce of your video library by converting it to efficient video codec like h265.

TranscoderD have a server and a worker application. The server will manage and schedule the jobs, the worker will do the actual encoding.
You can have as many workers you want, locally or remote in multiple machines.

## Features
- Convert video files to h265
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
      ghcr.io/segator/transcoderd:server-v2.4.3 # x-release-please-version
```

### Grafana Statistics
- Add postgres as datasource for grafana.
- Import this [grafana-dashboard](./grafana-dashboard.json) and change the data source to your own Postgres database.


## Worker Installation
### Run
```bash
docker run -d \
       --name transcoderd-worker \
       --restart=always \
       -v /tmp:/tmp \ # Ensure to have enough space (+50G, but depends on your biggest media size) on your temporal folder, as the worker will use it heavily for encoding
       --hostname $(hostname) \ 
       ghcr.io/segator/transcoderd:worker-v2.4.3 \ # x-release-please-version
       --web.token my_secret_token \ # Replace it for the same value as in the server config
        --web.domain http://192.168.1.55:8080 # Replace it for the server IP or public endpoint if you want remote access.
        
```

## Development

The current behavior contracts are in [OpenSpec](./openspec/specs/). Start with the [baseline map and known gaps](./openspec/BASELINE.md), [configuration reference](./openspec/CONFIGURATION.md), and [HTTP protocol reference](./openspec/PROTOCOL.md) before changing a capability; contributor guidance is in [AGENTS.md](./AGENTS.md).

### Prerequisites
- [Go 1.26](https://go.dev/dl/)
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- Docker with [buildx](https://docs.docker.com/build/install-buildx/) (for container builds)
- PostgreSQL (for integration tests, provided via testcontainers)

Install [mise](https://mise.jdx.dev/) for Go, Mage, and Dagger tooling. Dagger runs builds and tests in reproducible containers.

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

The Dagger pipeline builds FFmpeg 9.0.2 with `libsvtav1`, `libaom-av1`, and
`av1_nvenc`, and builds the PGS assets used by the worker. These builds run in
containers and do not require a GPU on the build host. GPU encoding at runtime
requires a compatible NVIDIA GPU and driver exposed to the worker container.
On Linux, configure NVIDIA Container Toolkit and run with `--gpus all`. The
worker image has also been tested on an RTX 5080 under WSL2 using direct device
and driver mounts. See [AV1/NVENC validation](docs/av1-nvenc-validation.md).

```bash
mage docker:ffmpeg      # Build FFmpeg builder image
mage docker:pgs         # Build PGS asset image
mage publish:app        # Push server and worker images
```

### CI Pipeline

- **Lint**: runs `golangci-lint` on every push and PR
- **Build + Test + E2E**: runs Mage/Dagger tests, builds both Docker images, and runs Docker E2E on pushes and PRs
- **Docker Publish**: pushes server and worker images on `main`
- **Release**: managed by [Release Please](https://github.com/googleapis/release-please) on `main` using conventional commits

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
