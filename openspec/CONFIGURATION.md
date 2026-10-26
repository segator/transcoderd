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
| `worker.maxActiveJobs` | `2` | Startup FFmpeg encode target. The worker may admit up to two additional pipeline jobs beyond the effective encode target (including a positive `parallelJobs` override) for preparation and verification/upload overlap. A value saved by `control set-max-jobs N` in the server database takes precedence on subsequent starts; `0` disables new requests and leaves the last positive encode target in place. |
| `worker.ffmpegConfig.parallelJobs` | `0` | Initial FFmpeg encode target: inherits `worker.maxActiveJobs`; a positive advanced override sets the startup target. A positive runtime `control set-max-jobs N` replaces this target. |
| `worker.ffmpegConfig.threads` | `0` | Value passed to FFmpeg's `-threads` option; zero lets FFmpeg decide. |
| `worker.ffmpegConfig.videoCodec` | `av1_nvenc` | Selects the AV1 GPU profile, `libsvtav1` CPU profile, or explicit `libx265` rollback. |
| `worker.ffmpegConfig.videoPreset` | Empty | Profile default: NVENC `p7`, SVT `4`, x265 `medium`. A supplied value must be valid for the selected encoder. |
| `worker.ffmpegConfig.videoCQ` | `0` (profile default) | NVENC CQ; zero selects `38`. Invalid with CPU or x265. |
| `worker.ffmpegConfig.videoQMax` | `0` (profile default) | NVENC maximum QP; zero selects `100`. Paired with `-qmin 0`. Invalid with CPU or x265. |
| `worker.ffmpegConfig.videoCRF` | `0` (profile default) | SVT CRF `25` or x265 CRF `21` when omitted. Invalid with NVENC. |
| `worker.ffmpegConfig.videoProfile` | Empty | x265 `main10` when omitted. Invalid with AV1. |
| `worker.ffmpegConfig.audioCodec` | `libfdk_aac` | Audio encoder passed per selected stream as `-c:a:N`. |
| `worker.ffmpegConfig.audioVBR` | `5` | Audio quality passed per selected stream as `-vbr`. |
| `worker.ffmpegConfig.extraArgs` | Empty string | Inserted into the FFmpeg invocation before input files. |
| `worker.verifyDeltaTime` | `60` | Maximum allowed source/output video duration difference in seconds. Container mismatches are confirmed against video packet timelines. |
| `worker.startAfter` | `0s` | Optional local time elapsed since midnight before accepting work. |
| `worker.stopAfter` | `0s` | Optional local time elapsed since midnight after which work is not accepted. |
| `worker.schedule.timeZone` | `Local` | IANA timezone for advanced weekly admission windows; defaults to the worker process timezone. |
| `worker.schedule.windows` | Empty list | OR of `Days@HH:MM-HH:MM` intervals for new requests; start inclusive, end exclusive. |
| `--schedule` | No windows | Repeatable `HH:MM-HH:MM` or `Days@HH:MM-HH:MM` CLI shortcut using the process timezone; overrides YAML windows when supplied. |

`worker.threads` and `worker.ffmpegConfig.threads` are different fields. The former is registered but does not control FFmpeg. With no nested `threads` value, the command asks FFmpeg for `-threads 0`. SVT-AV1 also manages its own internal parallelism. `worker.maxActiveJobs` is the default simultaneous encode target. The worker may accept at most two additional end-to-end jobs beyond the effective encode target, so download and preparation can run ahead and verification/upload can overlap encoding without an unbounded queue. Each non-FFmpeg phase (download, MKV extraction, PGS, verification, and upload) has one job consumer. The optional `worker.ffmpegConfig.parallelJobs` value sets only the initial encode target; zero inherits `worker.maxActiveJobs`. A positive runtime `control set-max-jobs N` changes the encode target and the bounded admission ceiling together and persists the desired value in the server database under `worker.name`. Use zero to stop new requests and a positive value to resume; the server value overrides the startup flag after restart. Before any value has been saved, the startup configuration applies. Setting zero preserves the last positive encode target, so accepted jobs can continue through encoding. The updater's temporary drain-to-zero is not persisted. Lowering a positive target does not interrupt running encodes; queued encodes wait for an available slot. `control status` reports `max_jobs` and the separate `admitted_limit`. This is a concurrency limit, not a promise of linearly higher throughput.

The worker image bundles FFmpeg 9.0.2 with `libsvtav1`, `libaom-av1`, and `av1_nvenc`. The default now requires an NVIDIA GPU available to the worker container. On Linux, configure the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html) and pass `--gpus all`; WSL2 may require the validated `/dev/dxg` driver mounts described in [the RTX 5080 validation](../docs/av1-nvenc-validation.md). For CPU-only workers select `libsvtav1` explicitly. Invalid combinations fail at worker startup, before accepting jobs. The exact approved full-film argument vector is in [the pilot record](../docs/av1-gpu-full-film-command.md).

The FFmpeg command maps the first video stream, scales width to `min(1920, input width)` while preserving aspect ratio, and writes an `-encoded.mkv` file. It maps one selected audio stream per language, choosing channel count first and bit rate second. It disables chapter copying. `extraArgs` is included before input arguments, so the meaning of an option depends on FFmpeg's option-placement rules.

### Weekly job admission

Use `TZ=Europe/Madrid` and repeatable `--schedule` arguments for daily or
day-selected admission windows. For example, `--schedule=22:00-10:00` permits
new requests every day from 22:00 until 10:00 in the process timezone. Select
days with `Days@`, such as `--schedule=Mon-Fri@22:00-24:00` and
`--schedule=Sat-Sun@00:00-24:00`. Repeated flags form an OR. This top-level
shortcut replaces `worker.schedule.windows` from YAML when supplied. Selectors
accept `*`, a three-letter weekday, or an inclusive weekday range. Overnight
windows belong to their starting day, starts are inclusive, and ends exclusive.
Split weekday morning and evening windows at midnight when both should be
allowed only on weekdays. `24:00` is allowed only as an end. Civil clock time
follows timezone/DST transitions; timezone data is bundled. Empty windows
disable the restriction.

The advanced settings have CLI flags; `--schedule=` clears configured windows.
Invalid entries, invalid zones, or combining weekly windows with active legacy
`startAfter`/`stopAfter` settings fail startup before server contact. Legacy
fields keep their existing duration semantics. Closing admission leaves accepted,
queued, and restored jobs running; requests initiated before the cutoff can
complete afterward. There is no tariff-price lookup or public-holiday calendar.
Schedule changes require restarting the worker. The active-job maximum can also
be changed at runtime with the local `control set-max-jobs` command. See the
[complete scheduling reference](../docs/worker-scheduling.md) for examples and
overrun implications.

### Current FFmpeg invocation

The command below is a schematic transcription of `worker/step/ffmpegStep.go`, with paths and stream indices left as placeholders. It is an implementation reference; the requirements in `specs/media-encoding/` describe the behavior that must remain stable.

```text
ffmpeg -fflags +genpts -nostats <extraArgs> -progress pipe:1 -hide_banner -nostdin \
  -threads <ffmpegConfig.threads> -analyzeduration 2147483647 -probesize 2147483647 \
  -i <downloaded-source> [-i <converted-subtitle.srt> ...] \
  -max_muxing_queue_size 9999 \
  -map 0:<video-stream> -avoid_negative_ts make_zero -copyts -map_chapters -1 \
  -flags +global_header \
  -filter:v "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease" \
  <selected video profile below> \
  [-map 0:<audio-stream> -metadata:s:a:<index> "title=<language> (<layout>)" \
    -c:a:<index> <audioCodec> -vbr <audioVBR> ...] \
  [subtitle mappings and dispositions] -y <workspace>/<source-stem>-encoded.mkv
```

The video portion is one of:

```text
AV1 GPU: -pix_fmt p010le -c:v av1_nvenc -preset p7 -tune uhq -rc vbr
         -cq 38 -b:v 0 -multipass fullres -bf 4 -b_ref_mode middle
         -split_encode_mode disabled -qmin 0 -qmax 100
AV1 CPU: -pix_fmt yuv420p10le -c:v libsvtav1 -preset 4 -crf 25
HEVC:    -pix_fmt yuv420p10le -c:v libx265 -crf 21
         -profile:v main10 -preset medium
```

The output is probed, hashed with SHA-256, and measured in bytes. Duration verification uses `worker.verifyDeltaTime`; an encoded MKV larger than its source is rejected before upload. No VMAF prepass or automatic retry is required. Current defaults are the fixed GPU p7/UHQ/CQ38 profile and SVT preset 4 / CRF25; neither guarantees a p5 VMAF target for every title. Three representative 15-second clips showed 18–49% smaller video outputs at these candidate settings, with p5 VMAF changes from -0.13 to -2.37 for NVENC and -0.48 to -1.37 for SVT. Grain, motion, resolution, source codec, and scene complexity change the result; short clips do not predict every full film.

### Example worker YAML

```yaml
web:
  domain: http://transcoderd-server:8080
  token: replace-this-token
worker:
  name: encode-node-1
  temporalPath: /tmp
  maxActiveJobs: 2
  verifyDeltaTime: 60
  ffmpegConfig:
    threads: 0
    videoCodec: av1_nvenc
    parallelJobs: 2
    videoCQ: 38
    videoQMax: 100
    audioCodec: libfdk_aac
    audioVBR: 5
    extraArgs: ""
  pgsConfig:
    parallelJobs: 2
```

For a CPU worker set `videoCodec: libsvtav1`; the default is preset `4` / CRF `25`. Set `parallelJobs: 1` if a single SVT encode should have the CPU to itself. For x265 rollback set `videoCodec: libx265`, `videoPreset: medium`, `videoProfile: main10`, and `videoCRF: 21`. Remove NVENC-only `videoCQ` and `videoQMax` from either alternative profile. The separate `worker.threads` flag does not drive FFmpeg. The worker image supplies FFmpeg, ffprobe, mkvextract, .NET, PgsToSrt, and Tesseract assets.

### Separate CPU and GPU workers

Run two worker instances using the same server URL, credential, and schedule.
Their node-specific encoder arguments are:

```text
GPU: --worker.name gpu-5080 --worker.maxActiveJobs 4 --worker.ffmpegConfig.videoCodec av1_nvenc
CPU: --worker.name cpu-9900x3d --worker.maxActiveJobs 2 --worker.ffmpegConfig.videoCodec libsvtav1
```

Omitted video settings resolve to the selected encoder's defaults. Omitted
`parallelJobs` follows `maxActiveJobs`, so no second startup concurrency
setting is needed. Runtime `control set-max-jobs N` changes both job admission
and FFmpeg encode concurrency; zero stops new assignments but leaves the last
positive encode target in place. Each non-FFmpeg phase processes one job at a
time per worker. Preset/quality overrides remain available for benchmarks and advanced
use. Shared `TZ=Europe/Madrid --schedule=22:00-10:00` arguments permit new
requests nightly on every day, including weekends. See
[the deployment guide](../docs/worker-deployment.md) for complete CLI-only Docker
commands and scratch/GPU access. YAML examples above remain supported for
existing installations; no YAML file is required.

Both workers request jobs from the same server. Job assignment is based on
availability, so this does not route a particular movie or source codec to a
particular encoder. The limits apply per worker instance; four GPU and two CPU
slots permit up to six active jobs total. On the RTX 5080 and Ryzen 9900X3D,
a repeated 120-second 1080p clip measured aggregate p7/UHQ GPU throughput of
5.74 source seconds per wall second with one job and 10.61 with four. SVT
preset 3 rose from 1.88 to 2.20 with two jobs. Thus four GPU jobs nearly
doubled total throughput in that trial, while two CPU jobs added about 17%; no
single job became faster. Tune these values against real titles, available
memory, OCR, transfer traffic, and power. On WSL2, preserve the NVIDIA driver
library path in the GPU container; the worker passes its environment to FFmpeg.

The previous HEVC command and defaults remain in the
[legacy reference](../docs/legacy-hevc-encoding.md).

The [worker deployment guide](../docs/worker-deployment.md) covers NVIDIA driver
and container-runtime setup, Unraid/WSL differences, image builds, and complete
GPU/CPU deployment examples. The [encoding profiles](../docs/encoding-profiles.md)
explain each FFmpeg parameter, including audio and the CPU preset alternatives.

## PGS OCR settings

| Key | Registered default | Effect |
| --- | --- | --- |
| `worker.pgsConfig.parallelJobs` | `1` | Maximum parallel OCR actions for a PGS phase. Increase explicitly only when the worker has sufficient memory. |
| `worker.pgsConfig.dotnetPath` | `dotnet` | .NET executable for PgsToSrt. |
| `worker.pgsConfig.executablePath` | Empty, or bundled `pgs/PgsToSrt.exe` on Windows | Direct OCR executable; when set, supersedes the dotnet/assembly invocation. An explicit empty value restores the legacy invocation. |
| `worker.pgsConfig.DLLPath` | `./PgsToSrt.dll` | OCR assembly. |
| `worker.pgsConfig.tessdataPath` | `./tessdata`, or bundled `tessdata` on Windows | Tesseract language data; bundled paths are resolved relative to the worker executable. |
| `worker.pgsConfig.tessVersion` | `5` | Tesseract version passed to PgsToSrt. |
| `worker.pgsConfig.libleptName` | `leptonica` | Leptonica library name. |
| `worker.pgsConfig.libleptVersion` | `6` | Leptonica version passed to PgsToSrt. |

The OCR language is derived from each subtitle stream's language metadata. The worker maps common two- and three-letter codes for several languages and otherwise passes through the source code; an empty language falls back to English.
