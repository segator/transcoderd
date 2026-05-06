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

### Prerequisites
- [mise](https://mise.jdx.dev/) + [direnv](https://direnv.net/) (auto-installs Go, Mage, Dagger on `cd`)
- Docker


### Build

```bash
mage build:all          # Build server + worker Go binaries -> dist/
mage build:server       # Build server binary only
mage build:worker       # Build worker binary only
```

### Test

```bash
mage test:unit          # Unit tests with coverage
mage test:race          # Unit tests with race detector
mage test:short         # Unit tests in short mode
mage test:integration   # Integration tests (via Dagger services)
mage test:all           # Unit + integration tests
mage test:e2e           # Full end-to-end test (server + worker + postgres in Dagger)
```

### Lint

```bash
mage lint:check         # Run golangci-lint
mage lint:fix           # Run golangci-lint with auto-fix
mage lint:fmt           # Run go fmt
```

### Docker (via Dagger)

Container images are built using [Dagger](https://dagger.io/) for content-addressed caching.

```bash
mage docker:all         # Build server + worker Docker images (loads locally)
mage docker:server      # Build server Docker image only
mage docker:worker      # Build worker Docker image only
mage docker:ffmpeg      # Build FFmpeg builder image locally (~50min)
mage docker:pgs         # Build PGS builder image locally (~5min)
```

### Publish

```bash
mage publish:app        # Push server + worker images to registry
mage publish:ffmpeg     # Push FFmpeg builder image
mage publish:pgs        # Push PGS builder image
mage publish:all        # Push all images
```

### CI Pipeline

- **Lint**: runs `golangci-lint` on every push and PR
- **Build + Test**: runs `mage test` and `mage build` on every push and PR
- **Docker**: builds container images via Dagger on every push and PR
- **Docker Publish**: pushes Docker images on pushes to `main` only (PRs skip publish)
- **Release**: managed by [Release Please](https://github.com/googleapis/release-please) on `main` using conventional commits

### Project Structure

```
cmd/            # Shared CLI config (pflag, viper, mapstructure)
model/          # Domain types (Job, Event, Worker)
helper/         # Utilities (command exec, concurrent collections, progress)
magefiles/      # Mage build targets (Dagger-powered container builds)
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
