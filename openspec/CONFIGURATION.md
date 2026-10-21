# Configuration reference

This reference records the configuration accepted by the current binaries. The OpenSpec capability contracts are in `specs/configuration-and-runtime/`, `specs/media-encoding/`, and `specs/subtitles/`. Values below come from the registered CLI defaults in `cmd/flags.go`, `server/main.go`, and `worker/main.go`, plus fields read by their consumers.

## Configuration sources

Both binaries register CLI flags, read YAML through Viper, and enable environment lookup with the `TR_` prefix. Configuration files are searched in `/etc/transcoderd/`, `$HOME/.transcoderd/`, and the current directory. CLI flags override values read from the file. The application does not install a Viper environment-key replacer; nested environment variable spellings such as `TR_WORKER_FFMPEGCONFIG_VIDEOCRF` have not been established as a supported interface. Use YAML or CLI flags for nested values unless the mapping is verified.

The `envconfig` struct tags are not consumed by the Viper setup. They document field intent, but are not evidence that the corresponding environment variable works.

Durations such as `5m` and `24h` are parsed as Go durations. `worker.verifyDeltaTime` is a number of seconds, not a duration string. `server.database.scheme` is passed as the PostgreSQL database name (`dbname`).

## Shared and server settings

| Key | Registered default | Effect |
| --- | --- | --- |
| `web.port` | `8080` | Server HTTP listen port. |
| `web.token` | `admin` | Bearer token for job and event control routes. |
| `web.domain` | `http://localhost:8080` | Base URL used by workers for requests and media transfer. |
| `updateCheckInterval` | `15m` | Minimum interval between release queries. |
| `noUpdates` | `false` | Disable executable replacement. |
| `verbose` | `false` | Enable debug logging. |
| `server.scheduler.sourcePath` | `/data/current` | Absolute root containing source files and receiving encoded targets. |
| `server.scheduler.scheduleTime` | `5m` | Job-maintenance interval. |
| `server.scheduler.jobTimeout` | `24h` | Age threshold for rescheduling assigned or started jobs. |
| `server.scheduler.minFileSize` | `100000000` | Minimum source size in bytes. |
| `server.scheduler.deleteOnComplete` | `false` when omitted | Delete the original after successful completion processing. This field is configured through YAML; no flag is registered. |
| `server.database.driver` | `postgres` | SQL driver. |
| `server.database.host` | `localhost` | PostgreSQL host. |
| `server.database.port` | `5432` | PostgreSQL port. |
| `server.database.user` | `postgres` | PostgreSQL user. |
| `server.database.password` | `postgres` | PostgreSQL password. |
| `server.database.scheme` | `server` | PostgreSQL database name. |

The four control routes (`POST /api/v1/job/`, `DELETE /api/v1/job/{jobid}`, `GET /api/v1/job/request`, and `POST /api/v1/event`) check `Authorization: Bearer <web.token>`. The media routes do not use that middleware; see `BASELINE.md`.

### Example server YAML

```yaml
web:
  port: 8080
  token: replace-this-token
server:
  database:
    driver: postgres
    host: postgres
    port: 5432
    user: transcoder
    password: replace-this-password
    scheme: transcoderd
  scheduler:
    sourcePath: /media
    scheduleTime: 5m
    jobTimeout: 24h
    minFileSize: 100000000
    deleteOnComplete: false
```

## Worker and FFmpeg settings

| Key | Registered default | Effect |
| --- | --- | --- |
| `worker.name` | Hostname | Worker identity used in events and assignment. |
| `worker.temporalPath` | OS temporary directory | Parent of `worker-<name>` workspaces. |
| `worker.threads` | Number of CPUs, capped at 16 | Registered flag; the current encoder does not read this field. See `BASELINE.md`. |
| `worker.ffmpegConfig.threads` | `0` when omitted | Value passed to FFmpeg's `-threads` option. This nested field has no registered CLI flag, but is mapped from YAML. |
| `worker.ffmpegConfig.videoCodec` | `libx265` | Video encoder passed as `-c:v`. |
| `worker.ffmpegConfig.videoCRF` | `21` | Video quality passed as `-crf`. |
| `worker.ffmpegConfig.videoProfile` | `main10` | Video profile passed as `-profile:v`. |
| `worker.ffmpegConfig.videoPreset` | `medium` | Encoder preset passed as `-preset`. |
| `worker.ffmpegConfig.audioCodec` | `libfdk_aac` | Audio encoder passed per selected stream as `-c:a:N`. |
| `worker.ffmpegConfig.audioVBR` | `5` | Audio quality passed per selected stream as `-vbr`. |
| `worker.ffmpegConfig.extraArgs` | Empty string | Inserted into the FFmpeg invocation before input files. |
| `worker.verifyDeltaTime` | `60` | Maximum absolute source/output duration difference in seconds. |
| `worker.startAfter` | `0s` | Optional local time elapsed since midnight before accepting work. |
| `worker.stopAfter` | `0s` | Optional local time elapsed since midnight after which work is not accepted. |
| `worker.paused` | `false` when omitted | Disables requests for new jobs; no CLI flag is registered. |

`worker.threads` and `worker.ffmpegConfig.threads` are different fields. The former is registered in `worker/main.go` but not copied to the latter. With no nested `threads` value, the current command asks FFmpeg for `-threads 0`. This is an observed wiring gap, not a deliberate policy that agents should preserve.

The worker image bundles FFmpeg 9.0.2 with `libsvtav1`, `libaom-av1`, and `av1_nvenc` available for future AV1 experiments. The job pipeline still uses HEVC-oriented defaults (`libx265`, CRF `21`, profile `main10`, preset `medium`, and `yuv420p10le`). Setting `videoCodec` alone does not establish a validated AV1 mode; encoder-specific profiles, presets, quality settings, and GPU access need separate work.

The FFmpeg command maps the first video stream, uses `yuv420p10le`, scales width to `min(1920, input width)` while preserving aspect ratio, and writes an `-encoded.mkv` file. It maps one selected audio stream per language, choosing channel count first and bit rate second. It disables chapter copying. `extraArgs` is included before input arguments, so the meaning of an option depends on FFmpeg's option-placement rules.

### Current FFmpeg invocation

The command below is a schematic transcription of `worker/step/ffmpegStep.go`, with paths and stream indices left as placeholders. It is an implementation reference; the requirements in `specs/media-encoding/` describe the behavior that must remain stable.

```text
ffmpeg -fflags +genpts -nostats <extraArgs> -progress pipe:1 -hide_banner \
  -threads <ffmpegConfig.threads> -analyzeduration 2147483647 -probesize 2147483647 \
  -i <downloaded-source> [-i <converted-subtitle.srt> ...] \
  -max_muxing_queue_size 9999 \
  -map 0:<video-stream> -avoid_negative_ts make_zero -copyts -map_chapters -1 \
  -flags +global_header \
  -filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease" \
  -pix_fmt yuv420p10le -c:v <videoCodec> -crf <videoCRF> \
  -profile:v <videoProfile> -preset <videoPreset> \
  [-map 0:<audio-stream> -metadata:s:a:<index> "title=<language> (<layout>)" \
    -c:a:<index> <audioCodec> -vbr <audioVBR> ...] \
  [subtitle mappings and dispositions] <workspace>/<source-stem>-encoded.mkv -y
```

The output is probed again and hashed with SHA-256. Duration verification uses `worker.verifyDeltaTime` before upload. The code also attempts an output-size check, but currently does not populate the target size; see `BASELINE.md`.

### Example worker YAML

```yaml
web:
  domain: http://transcoderd-server:8080
  token: replace-this-token
worker:
  name: encode-node-1
  temporalPath: /tmp
  verifyDeltaTime: 60
  ffmpegConfig:
    threads: 4
    videoCodec: libx265
    videoCRF: 21
    videoProfile: main10
    videoPreset: medium
    audioCodec: libfdk_aac
    audioVBR: 5
    extraArgs: ""
  pgsConfig:
    parallelJobs: 2
```

The example explicitly sets `worker.ffmpegConfig.threads` because the separate `worker.threads` flag does not currently drive FFmpeg. A worker also needs the FFmpeg, ffprobe, mkvextract, .NET, PgsToSrt, and Tesseract assets used by the selected media; the worker image supplies them.

## PGS OCR settings

| Key | Registered default | Effect |
| --- | --- | --- |
| `worker.pgsConfig.parallelJobs` | `ceil(CPU count / 4)` | Maximum parallel OCR actions for a PGS phase. |
| `worker.pgsConfig.dotnetPath` | `dotnet` | .NET executable for PgsToSrt. |
| `worker.pgsConfig.DLLPath` | `./PgsToSrt.dll` | OCR assembly. |
| `worker.pgsConfig.tessdataPath` | `./tessdata` | Tesseract language data. |
| `worker.pgsConfig.tessVersion` | `5` | Tesseract version passed to PgsToSrt. |
| `worker.pgsConfig.libleptName` | `leptonica` | Leptonica library name. |
| `worker.pgsConfig.libleptVersion` | `6` | Leptonica version passed to PgsToSrt. |

The OCR language is derived from each subtitle stream's language metadata. The worker maps common two- and three-letter codes for several languages and otherwise passes through the source code; an empty language falls back to English.
