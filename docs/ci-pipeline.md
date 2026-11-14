# CI pipeline and build cache

## Jobs and execution platforms

The `source` job resolves the immutable commit and version. Linux checks,
Windows distribution packaging and Windows update-fixture packaging start
independently from those outputs. The required `ci` status aggregates all three
build results. Superseded runs cancel within each PR. Main and release
completion runs continue independently.

The `windows-native` job consumes `windows-worker` and validates the full
portable ZIP, desktop installer, native unit tests, media tools, OCR and CPU
E2E. The `windows-update-native` job consumes `windows-update` and starts as soon
as its own fixture and harness are available. The two jobs use separate
`windows-2025` machines for their installed services and recovery tasks.

Update fixtures use the same installer and real SVT-AV1 encoding and OCR.
PR fixtures contain the English and Spanish traineddata used by the E2E input.
Pushes to main and manual runs validate fixtures with the full payload.
Completion of an older release runs the Mage targets from the validated SHA
and extracts a small native update harness from that checkout's outputs.

The read-only `windows-debug.yml` workflow is manual. It can use older combined
artifacts or the separate distribution and update artifacts. Its execution
uses the current PowerShell harness and has no release publication path.

## Engine cache and compiler budget

Mage orchestrates targets; Dagger executes their containers on the connected
Engine. The GitHub runner's CPU count is not the compiler host's CPU count.
The observed CI jobs connect to `dagger-engine-dagger-helm-engine-0`. A cache
belongs to that Engine; another Engine needs persisted/shared state of its own.
Installing `dagger-for-github` alone does not configure cache persistence.

`WINDOWS_BUILD_JOBS` is a positive-integer **aggregate C/C++ compiler budget**,
default **16**, matching the current Engine. Up to four dependency evaluations
run concurrently, each with `floor(budget / min(4, budget))` compiler jobs.
Budgets of one or two remain bounded. FFmpeg runs after the dependency builds
and receives the entire budget. This setting does not affect encoding slots or
FFmpeg runtime threads.

AOM, SVT, x265, FDK AAC and NVCodec each start from the same MinGW base and import
only their own stage script plus the common build helpers. Their installed
prefixes and licenses merge into FFmpeg's final compilation. An SVT stage edit
therefore rebuilds SVT and FFmpeg; it leaves unrelated libraries cached. A shared
helper or compiler-base change correctly invalidates all dependent stages.

Windows toolchain source GETs retry transport errors, including TLS resets and
partial transfers, up to five times. Connection setup is limited to 30 seconds,
each attempt to 120 seconds, and new retries to a 300-second window (an attempt
already in progress can finish after that window). Curl writes to a regular
temporary file so a partial attempt is discarded before retrying. Pinned SHA-256
verification runs before extraction; a checksum mismatch fails immediately.
The temporary archive is removed on success or failure. These retry limits
apply to source downloads, without rerunning compilers or whole CI jobs.

Go module and build caches remain mounted on the Engine. The portable media ZIP
is cached independently of the application worker. Dagger adds the linked
worker to the cached ZIP for each build. The distribution target evaluates a
maximum of two packaging or cross-compilation outputs at once. Update fixtures
share the compiler, native harness, published desktop UI and downloaded
prerequisites where their inputs match.

## Images, artifacts and publication

CI uses `mage docker:check` to evaluate application-image graphs without exporting
multi-gigabyte media image archives or loading the runner's Docker daemon.
Frontend checks inspect a small temporary OCI index containing Linux amd64 and
arm64 and execute each runtime's version and health endpoints (with emulation
for the foreign CPU). Foreign service smoke tests carry a static emulator from the pinned Dagger
Engine image in a test-only container. This works without host binfmt setup;
the published frontend images keep their original executable entrypoints and
contain no emulator.
Frontend publication and archives contain both variants. Local Docker loading
selects the host-compatible variant. Dagger Services run the CPU E2E.
`mage docker:all` exports and loads local images. The release pipeline uses
`mage release:linuxAssets` to assemble the same OCI archives and Linux binaries
and upload them from the Engine. The source guard selects `docker:all` when
a tagged checkout needs its historical image target.

Windows release publication waits for `ci`, both native Windows suites and
Linux publication when applicable. It checks release version and SHA, uploads
the validated payloads and manifest, then uploads the signature last.
Tagged completion preserves previous Linux release assets.

## Packaging evidence

Six successful PRs from November 5–13 had a median of 21m16s from the start
of `ci` through native Windows completion. Median Windows build time was 5m,
transfer took 4m16s, and the native update suite took 7m30s. The
[frontend PR](https://github.com/segator/transcoderd/actions/runs/43385151713)
transferred a 3.81 GB artifact. The split workflow reports each artifact size
and suite time separately. The update harness also logs the wall time of each
scenario.


The first optimized [PR run](https://github.com/segator/transcoderd/actions/runs/43584308152)
completed in 9m33s including startup queue, or 9m21s from the first job start
to native completion. The required check passed at 4m08s from workflow start.
The distribution artifact was 1,988,851,087 bytes and the update artifact was
572,924,655 bytes, down 33% combined from the previous 3.81 GB artifact.
Native distribution validation took 3m09s and the update E2E step took 5m17s.
These figures are one warm GitHub run and are sensitive to runner and network load.

Historical run [41876078723](https://github.com/segator/transcoderd/actions/runs/41876078723)
took 52m05s overall: Linux CI 1m37s, Windows build 35m24s and native Windows
14m23s. All encoder/OCR dependency builds hit cache. NSIS processed approximately
1.90 GB for each installer: production packaging took 14m47s, followed by two
fixture installers in parallel taking 14m43s. The uploaded artifact was
3,660,789,366 bytes. These are measured Actions timings, not cold-build estimates.

The optimized recipe uses zlib level 3 and retains NSIS datablock deduplication.
The portable ZIP also uses compression level 3.
It retains all installed files, OCR languages, runtime prerequisites and the
complete standalone installer format. Higher installer bytes are an acceptable
trade-off only when supported by the same-payload measurements; this setting
does not change encoded movie quality or size. Native installation/update tests
remain mandatory. Local packaging measurements and CI results are recorded
separately because CPU/storage and cache state differ.

Local measurements on 2026-11-04 used Debian trixie NSIS and the same 385-file,
1,669,865,257-byte pinned Windows media/OCR payload:

| Recipe | Packaging time | Installer bytes |
| --- | ---: | ---: |
| Previous default zlib/deduplication | 500.41 s | 762,704,148 |
| zlib level 3, deduplication off (control) | 33.32 s | 792,903,293 |
| zlib level 3, default deduplication (selected) | 34.75 s | 792,287,304 |

The selected single-run local comparison reduced packaging time by 93.1% and
increased bytes by 3.88%. Disabling deduplication saved only 1.43s and grew the
installer, so the production recipe keeps it enabled. Extracted payload files
were verified against their original SHA-256 digests.
The selected complete desktop installer, including UI and prerequisites, also
built successfully: its NSIS step took about 73s, produced 903,355,308 bytes and
retained 879 installed files with matching manifest digests. The warm portable
target completed in 36.7s. Two concurrent selected update-fixture installers
took about 72–74s. These local numbers are not a prediction of the whole GitHub
workflow duration.

The independent-dependency cross-build completed locally in 4m13s. A subsequent
temporary SVT-script edit rebuilt SVT and FFmpeg while AOM, x265, FDK and NVCodec
reported `CACHED`; the edit was restored after verification. Native bundled-tool
tests executed the extracted tools successfully on Windows, including AV1,
x265, AAC, probing, subtitle extraction and OCR.

## Maintenance

The scheduled stale-issue workflow and Dependabot auto-merge workflow remain
active. Checkout, upload/download artifacts and stale actions use verified
Node-24 releases, compatible with the observed runner 2.338.2. Maintenance
permissions are explicit. The unused `act` event JSON and an unconsumed Dependabot metadata step
were removed. `build/tools/actionlint` with `.github/actionlint.yaml` recognizes
the custom `transcoderd` runner label; release-source Python tests and Mage tests
cover source guards, compiler budgets and publication separation.
