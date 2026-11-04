# Configuration reference

This reference records the configuration accepted by the current binaries. The OpenSpec capability contracts are in `specs/configuration-and-runtime/`, `specs/media-encoding/`, and `specs/subtitles/`. Values below come from the registered CLI defaults in `cmd/flags.go`, `server/main.go`, and `worker/main.go`, plus fields read by their consumers.

## Configuration sources

Both binaries register CLI flags, read YAML through Viper, and enable environment lookup with the `TR_` prefix. Configuration files are searched in `/etc/transcoderd/`, `$HOME/.transcoderd/`, and the current directory. CLI flags override values read from the file. The application does not install a Viper environment-key replacer; nested environment variable spellings such as `TR_WORKER_FFMPEGCONFIG_VIDEOCRF` have not been established as a supported interface. Use YAML or CLI flags for nested values unless the mapping is verified.

The `envconfig` struct tags are not consumed by the Viper setup. They document field intent, but are not evidence that the corresponding environment variable works. Dashboard settings explicitly bind `TR_WORKER_DASHBOARD_LISTEN_ADDRESS`, `TR_WORKER_DASHBOARD_TOKEN` and the server login credential `TR_WEB_DASHBOARD_TOKEN`.

Durations such as `5m` and `24h` are parsed as Go durations. `worker.verifyDeltaTime` is a number of seconds, not a duration string. `server.database.scheme` is passed as the PostgreSQL database name (`dbname`).

## Shared and server settings

| Key | Registered default | Effect |
| --- | --- | --- |
| `web.port` | `8080` | Server HTTP listen port. |
| `web.token` | `admin` | Bearer token for job and event control routes. |
| `web.dashboardToken` | `encoding123` | API-owned dashboard login credential; token-mode user token or private OIDC frontend login credential. Explicit env TR_WEB_DASHBOARD_TOKEN. Separate from worker API credentials. |
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

Job, event, and worker-settings control routes check `Authorization: Bearer <web.token>`. The media routes do not use that middleware; see `BASELINE.md`. The independent frontend serves `/dashboard/` and forwards dashboard API calls to the server. The API uses `web.dashboardToken`, not `web.token`. Its browser session lasts 12 hours; logout or a server restart revokes it. See [dashboard operations](../docs/dashboard.md) for configuration, metric definitions, and worker rollout.

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
| `worker.ramPaused` | `false` | Startup preference for the independent RAM encoding hold. It blocks requests and encode starts without changing limits; it is not a durable encoder checkpoint. |
| `worker.pauseMode` | `finish` | `finish` lets active encodes finish on pause; `ram` suspends owned FFmpeg encoders on Windows/Linux and retains RAM/VRAM until Resume. Neither process exit nor reboot preserves suspended state. |
| `worker.temporalPath` | OS temporary directory | Parent of `worker-<name>` workspaces. |
| `worker.threads` | Number of CPUs, capped at 16 | Registered flag; the current encoder does not read this field. See `BASELINE.md`. |
| `worker.maxActiveJobs` | `2` | Startup FFmpeg encode target. The worker may admit up to one additional pipeline job beyond the effective encode target (including a positive `parallelJobs` override) for preparation and verification/upload overlap. A value saved by `control set-max-jobs N` in the server database takes precedence on subsequent starts; `0` pauses new requests and queued encodes while active encodes follow `worker.pauseMode` and downstream work finishes; the last positive encode target is retained. Windows service profiles instead retain the desktop limits and synchronize them to the server before admission. |
| `worker.dashboard.listenAddress` | Empty (disabled) | Optional local HTTP dashboard address, for example `127.0.0.1:8090` or `0.0.0.0:8090` inside Docker. Requires a dashboard token. YAML, `--worker.dashboard.listenAddress`, or `TR_WORKER_DASHBOARD_LISTEN_ADDRESS`. |
| `worker.dashboard.token` | Empty | Bearer token for the optional local dashboard API. Required when a dashboard listen address is configured. YAML, `--worker.dashboard.token`, or `TR_WORKER_DASHBOARD_TOKEN`. |
| `worker.ffmpegConfig.parallelJobs` | `0` | Initial FFmpeg encode target: inherits `worker.maxActiveJobs`; a positive advanced override sets the startup target. A positive runtime `control set-max-jobs N` replaces this target. |
| `worker.ffmpegConfig.threads` | `0` | Value passed to FFmpeg's `-threads` option; zero lets FFmpeg decide. |
| `worker.ffmpegConfig.videoCodec` | `av1_nvenc` | Selects the AV1 GPU profile, `libsvtav1` CPU profile, or explicit `libx265` rollback. |
| `worker.ffmpegConfig.videoPreset` | Empty | Profile default: NVENC `p7`, SVT `4`, x265 `medium`. A supplied value must be valid for the selected encoder. |
| `worker.ffmpegConfig.videoCQ` | `0` (profile default) | NVENC CQ; zero selects `38`. Invalid with CPU or x265. |
| `worker.ffmpegConfig.videoQMax` | `0` (profile default) | NVENC maximum QP; zero selects `100`. Paired with `-qmin 0`. Invalid with CPU or x265. |
| `worker.ffmpegConfig.videoCRF` | `0` (profile default) | SVT CRF `26` or x265 CRF `21` when omitted. Invalid with NVENC. |
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

`worker.threads` and `worker.ffmpegConfig.threads` are different fields. The former is registered but does not control FFmpeg. With no nested `threads` value, the command asks FFmpeg for `-threads 0`. SVT-AV1 also manages its own internal parallelism. `worker.maxActiveJobs` is the default simultaneous encode target. The worker may accept at most one additional end-to-end job beyond the effective encode target, so download and preparation can run ahead and verification/upload can overlap encoding without an unbounded queue. Each non-FFmpeg phase (download, MKV extraction, PGS, verification, and upload) has one job consumer. The optional `worker.ffmpegConfig.parallelJobs` value sets only the initial encode target; zero inherits `worker.maxActiveJobs`. A positive runtime `control set-max-jobs N` changes the encode target and the bounded admission ceiling together and persists the desired value in the server database under `worker.name`. Use zero to stop new requests and queued encodes, and a positive value to resume. For standalone workers the server value overrides the startup flag after restart; Windows service profiles also restore stored server limits and keep their local defaults when absent. Before any value has been saved, the startup configuration applies. Setting zero retains the last positive encode target for Resume, but holds queued encodes while active encodes follow `worker.pauseMode` and downstream work finishes. The updater's temporary maintenance drain permits all accepted work to finish and is not persisted. Lowering a positive target does not interrupt running encodes; queued encodes wait for an available slot. `control status` reports `max_jobs` and the separate `admitted_limit`. This is a concurrency limit, not a promise of linearly higher throughput.

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
AV1 CPU: -pix_fmt yuv420p10le -c:v libsvtav1 -preset 4 -crf 26
HEVC:    -pix_fmt yuv420p10le -c:v libx265 -crf 21
         -profile:v main10 -preset medium
```

The output is probed, hashed with SHA-256, and measured in bytes. Duration verification uses `worker.verifyDeltaTime`; an encoded MKV larger than its source is rejected before upload. No VMAF prepass or automatic retry is required. Current defaults are the fixed GPU p7/UHQ/CQ38 profile and SVT preset 4 / CRF26; neither guarantees a p5 VMAF target for every title. On three 24-second film excerpts, SVT-AV1 4.3.0 / CRF26 used 14.3–16.0% fewer video bytes than 4.2.0 / CRF25, with mean VMAF reductions of 0.286–0.363 point. Grain, motion, resolution and scene complexity affect the result; short clips do not establish full-film quality. See the [version/profile comparison](../docs/av1-svt-4-3-comparison.md).

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

For a CPU worker set `videoCodec: libsvtav1`; the default is preset `4` / CRF `26`. Set `parallelJobs: 1` if a single SVT encode should have the CPU to itself. For x265 rollback set `videoCodec: libx265`, `videoPreset: medium`, `videoProfile: main10`, and `videoCRF: 21`. Remove NVENC-only `videoCQ` and `videoQMax` from either alternative profile. The separate `worker.threads` flag does not drive FFmpeg. The worker image supplies FFmpeg, ffprobe, mkvextract, .NET, PgsToSrt, and Tesseract assets.

The bundled FFmpeg remains version 9.0.2; Linux and Windows pin SVT-AV1 4.3.0
for `libsvtav1`. The CPU profile remains preset 4 / CRF26. The offline version
comparison and its time/size/quality boundaries are recorded in
[`docs/av1-svt-4-3-comparison.md`](../docs/av1-svt-4-3-comparison.md).

Explicit CRF25 remains supported. New SVT jobs save the resolved CRF in their
profile snapshot; saved legacy SVT profiles with CRF0 retain the previous
CRF25 value on resume.

### Independent CPU and GPU limits per worker

The server stores an optional atomic `cpuJobs`/`gpuJobs` pair under each stable worker name. A pair overrides single-limit admission: CPU 2 and GPU 3 allow two CPU and three GPU encodes concurrently in one worker process, plus one bounded preparation/verification pipeline job. Each class drains on decrease; zero disables new unassigned work for that class, and both zero hold admission and queued encode starts while running/downstream work finishes. Pinned restored jobs retain their class/profile and can use bounded drain capacity on a disabled class. Workers synchronize about every five seconds, including while paused. Legacy clients without the independent-encoder capability read aggregate zero when a pair exists, preventing them from assigning the sum to their only encoder.

The preferred startup profile remains `worker.ffmpegConfig`. The other class defaults to SVT-AV1 preset 4/CRF26 or NVENC p7/CQ38, retaining shared audio/thread settings and omitting the other class's quality flags and extraArgs. Optional YAML-only `worker.cpuFFmpegConfig` and `worker.gpuFFmpegConfig` override the corresponding profiles; a CPU profile accepts libsvtav1/libx265 and a GPU profile accepts av1_nvenc. Empty profile videoCodec selects that class's default. Invalid combinations fail startup. The server's dashboard controls concurrency, not profile quality.

```yaml
worker:
  name: mixed-5080
  ffmpegConfig:
    videoCodec: av1_nvenc
    audioCodec: libfdk_aac
    audioVBR: 5
  cpuFFmpegConfig:
    videoCodec: libsvtav1
    videoPreset: "4"
    videoCRF: 26
  gpuFFmpegConfig:
    videoCodec: av1_nvenc
    videoPreset: p7
    videoCQ: 38
```

Save `{"cpuJobs":2,"gpuJobs":3}` from the dashboard settings API or its Workers form. A missing pair preserves legacy startup settings. A positive local `control set-max-jobs N` clears a pair and returns to N slots on the preferred startup encoder. GPU slots require the NVIDIA runtime; no automatic backend fallback is performed. Per-job profile snapshots preserve encoder choice after restart.

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


### Dashboard media libraries and previews

The server source library is `server.scheduler.sourcePath` and remains the only queueable root. `web.mediaRoots` is an optional map of named read-only absolute roots; it defaults to empty. Example `web.mediaRoots.encoded: /target` or `--web.mediaRoots encoded=/target` exposes an existing output mount. IDs use lowercase ASCII letters/digits/underscore/hyphen, up to 32 characters; `source` is reserved. Invalid entries are ignored. Missing roots are labeled unavailable. `web.previewMaxStreams` defaults to 2; values below 1 use 2. Compatible playback uses bounded H.264/AAC conversion up to 720p, independently of worker encode slots. Original streaming supports byte ranges. Every media endpoint uses dashboard auth.

Scans default to 1000 probe candidates (allowed 1–5000), two probe slots with 15-second timeouts, and a 100000-video discovery ceiling. Batches require completed reviewed selections and a 1–5000 successful-job target. At most two operations run, at most eight are retained, completed handles expire after 30 minutes, and operations time out after two hours. Metadata persists in migration 009. See `docs/media-library.md` for all filter semantics, defaults, rooted playback and jvdivx mount examples.
## RAM pause control

The dashboard has two independent controls: **Stop after current / Resume queue** holds admission and queued encode starts while active encodes finish; **Pause encodes / Resume encodes** suspends owned CPU/GPU encoding processes immediately in RAM. Releasing one preserves the other and individual holds.

`control pause-encodes` and `control resume-encodes` change the immediate RAM hold without changing limits. `control set-admission-limit N` persists an explicit admission limit using finish-active policy. Desktop settings persist the queue and RAM preferences separately; generated managed workers use finish policy plus `worker.ramPaused` so restart recovery cannot conflate the controls. Empty/legacy finish desktop pause-strategy values migrate to RAM.

For standalone compatibility, `worker.pauseMode` remains `finish` by default and legacy `control set-max-jobs N` retains its zero-limit policy. The explicit actions are independent of that policy. Standalone live RAM actions last until worker exit unless the startup preference is supplied in configuration.

RAM pause retains memory, GPU allocations, occupied encode slots and open output files. It cannot preserve encoder state across worker exit, shutdown or reboot. The server assignment timeout remains in force. Maintenance releases live RAM and individual holds before draining accepted work without changing saved desktop preferences.


### Worker UI synchronization

Local HTTP `POST /api/worker/max-jobs` accepts either legacy `maxJobs` or the atomic `cpuJobs`/`gpuJobs` pair (total at most 1024); missing, mixed or incomplete settings are rejected. Local CLI `control set-encoder-jobs CPU GPU` shares the same server persistence. Settings writes complete before local runtime changes. Managed startup restores server overrides and keeps local service defaults when absent. Last committed writes win; worker polls and heartbeats run approximately every five seconds, fleet refresh every five seconds, local browser refresh every two seconds.

Runtime heartbeat data adds `paused`, `ramPaused`, `pauseMode`, `suspendedEncodes`, `finishing`, `draining`, `managedProfile` and bounded `jobs` entries (`id`, `phase`, `state`, `individuallyPaused`, `pausedByWorker`). These observations do not overwrite desired limits. RAM and individual holds survive capacity changes. Queue stop holds pending encodes; maintenance/finish allows accepted work to drain without reopening admission. Windows supervised profiles retain their encoder class and support up to 64 slots each; incompatible fleet pair writes return 400. See `docs/dashboard.md`.

## Independent fleet frontend

`transcoderd-frontend` serves all fleet HTML/assets/PWA content and proxies only dashboard APIs. TranscoderD serves no UI assets. The frontend has no server/database/media/tool initialization and needs network access to the API and optionally the OIDC provider. It uses its own YAML/CLI configuration with the same config.yaml search paths or an explicit `--config` file. CLI overrides environment and YAML.

| Key | Default | Explicit environment | Effect |
| --- | --- | --- | --- |
| `frontend.listen` | `:8081` | `TR_FRONTEND_LISTEN` | Independent HTTP listener. |
| `frontend.apiURL` | `http://localhost:8080` | `TR_FRONTEND_API_URL` | Fixed TranscoderD API origin. |
| `frontend.publicURL` | empty | `TR_FRONTEND_PUBLIC_URL` | External browser origin for mutation checks and Secure cookies behind TLS termination; unset uses direct request HTTP/TLS and Host, ignoring forwarded headers. |

Both URL settings accept HTTP(S) origins without credentials, non-root paths, query or fragment. Browser requests remain same-origin through the frontend API proxy, including cookies, media ranges and streaming. API authentication/sessions, library state, FFmpeg and job operations remain on the API server. Optional OIDC browser login runs on the frontend. Worker local UI settings are unaffected. See [dashboard deployment](../docs/dashboard.md#deploy-on-a-separate-host).


### Frontend authentication modes

`frontend.auth.mode` defaults to `token`, preserving the API-owned single-user token login and twelve-hour session. In `oidc` mode, configure `frontend.apiToken` (matching API `web.dashboardToken`, at least 32 characters), `frontend.auth.sessionKey` (standard base64 of 32 random bytes), and `frontend.auth.oidc.issuerURL`, `clientID`, `clientSecret`. `scopes` defaults to `[openid, profile, email]`; openid is always requested. `allowedGroups` defaults to empty (provider application policy); a configured list requires any matching signed `groups` value. OIDC requires publicURL, secure provider/public endpoints (HTTP permitted only for loopback development), successful discovery and complete secrets; invalid settings fail startup without downgrade.

Explicit frontend environment names are `TR_FRONTEND_AUTH_MODE`, `TR_FRONTEND_API_TOKEN`, `TR_FRONTEND_AUTH_SESSION_KEY`, `TR_FRONTEND_AUTH_OIDC_ISSUER_URL`, `TR_FRONTEND_AUTH_OIDC_CLIENT_ID`, `TR_FRONTEND_AUTH_OIDC_CLIENT_SECRET`, `TR_FRONTEND_AUTH_OIDC_SCOPES` and `TR_FRONTEND_AUTH_OIDC_ALLOWED_GROUPS`. Array environment/CLI values are comma-separated. Equivalent `--frontend.*` flags override environment and YAML. The server additionally binds `TR_WEB_DASHBOARD_TOKEN` with normal CLI precedence.

The OIDC callback is `<frontend.publicURL>/dashboard/auth/callback`. Frontend replicas share the session key and API instance. API-owned sessions remain revocable; the browser's frontend cookie encrypts the opaque API session and never stores provider tokens or the API login credential. OIDC cookies are HttpOnly SameSite=Lax, with Secure for HTTPS; token-mode API cookies remain HttpOnly SameSite=Strict. Token fallback is disabled in OIDC mode. See [authentication, Authentik/Google and k3s examples](../docs/frontend-authentication.md).
