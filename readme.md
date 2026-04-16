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
      ghcr.io/segator/transcoderd:server-v2.4.1 # x-release-please-version
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
       ghcr.io/segator/transcoderd:worker-v2.4.1 \ # x-release-please-version
       --web.token my_secret_token \ # Replace it for the same value as in the server config
        --web.domain http://192.168.1.55:8080 # Replace it for the server IP or public endpoint if you want remote access.
        
```

## Development

### Prerequisites
- [Go 1.25](https://go.dev/dl/)
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- Docker with [buildx](https://docs.docker.com/build/install-buildx/) (for container builds)
- PostgreSQL (for integration tests, provided via testcontainers)

Alternatively, install [Devbox](https://www.jetify.com/devbox) which provides all Go and lint tooling via Nix.

### Build

```bash
make build              # Build Go binaries + Docker containers
make buildgo            # Build Go binaries only (no Docker)
make buildgo-server     # Build server binary only -> dist/transcoderd-server
make buildgo-worker     # Build worker binary only -> dist/transcoderd-worker
```

### Test

```bash
make test               # Unit tests with coverage
make test-race          # Unit tests with race detector
make test-short         # Unit tests in short mode
make test-integration   # Integration tests (requires Docker for testcontainers)
make test-all           # Unit + integration tests
make test-coverage      # Run tests and open coverage report in browser

# Run a single test
go test -v -run TestFunctionName ./path/to/package/...
```

### Lint

```bash
make lint               # Run golangci-lint
make lint-fix           # Run golangci-lint with auto-fix
make fmt                # Run go fmt
```

### Docker

```bash
make buildcontainer-server   # Build server Docker image (loads locally)
make buildcontainer-worker   # Build worker Docker image (loads locally)
make publishcontainer-server # Build and push server image to registry
make publishcontainer-worker # Build and push worker image to registry
```

Cache images for FFmpeg and PGS builder stages (avoids ~50min FFmpeg rebuilds):

```bash
make buildcache              # Build and push all cache images
make buildcache-ffmpeg       # Build and push FFmpeg cache image only
make buildcache-pgs          # Build and push PGS cache image only
```

### CI Pipeline

- **Lint**: runs `golangci-lint` on every push and PR
- **Build + Test**: runs `make test` and `make buildgo` on every push and PR
- **Docker Publish**: builds and pushes Docker images on pushes to `main` only (PRs skip Docker)
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
