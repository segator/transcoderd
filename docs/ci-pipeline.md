# CI pipeline and build cache

## Jobs and execution platforms

`.github/workflows/main.yml` has two jobs: the required Linux-hosted `ci` check
and `windows-native`. CI invokes Mage/Dagger for immutable release-source guards,
PWA/resource/activity tests, Linux/Windows lint, Go tests, Linux binaries/images/
E2E, both frontend image architectures, Windows portable distribution, native
test executables, the desktop installer and two update/recovery fixtures. Each
Windows target has a separate Actions step and timing. Source resolution, web
tests and Windows units no longer allocate separate jobs.

PRs against every base branch run these checks, preserving the former standalone
lint workflow's coverage. Push/release publication remains restricted to `main`.

After CI succeeds, `windows-native` checks network isolation, downloads the
cross-built tools and executes the unit suites (including frontend), complete
portable, installed-service, desktop and mandatory-update/recovery checks on
`windows-2025`. Go unit executables are cross-compiled once in CI and run natively;
FFmpeg and the codecs are not rebuilt on Windows. Older tagged-source completion
uses a native Go frontend-test fallback because its original exporters lack
frontend test executables.

Superseded PR runs cancel only earlier runs for the same PR; main and release
completion runs do not cancel one another. Current CI tools are compiled into
the runner's temporary directory before switching to validated tagged source;
Python/GitHub CLI guards and Node 24 web tests run in cached Dagger containers.
This preserves the new tooling across legacy checkouts without a new Python or
Node dependency on the Linux runner.

`.github/workflows/windows-debug.yml` is manual-only. Its `artifact_run` input
selects trusted existing fixtures for the current PowerShell harness. Its token
is read-only and it has no signing/publication path. The debugger is separated
because it executes old artifacts with different permissions and does not
validate new application binaries; it does not appear in ordinary PR runs.

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

Go module/build caches remain mounted in the Engine. Media/OCR directories are
assembled before adding commit-specific application executables, so worker edits
do not invalidate the large tool-copy layers. Portable and installer recipes use
the same worker-binary recipe. Pinned installer prerequisites have
their own source-independent download graph, shared by production and fixture
installers. An application commit does not redownload them.

## Images, artifacts and publication

CI uses `mage docker:check` to evaluate application-image graphs without exporting
multi-gigabyte media image archives or loading the runner's Docker daemon.
Frontend checks inspect a small temporary OCI index containing Linux amd64 and
arm64 and execute each runtime's version and health endpoints (with emulation
for the foreign CPU). Foreign service smoke tests carry a static emulator from the pinned Dagger
Engine image in a test-only container. This works without host binfmt setup;
the published frontend images keep their original executable entrypoints and
contain no emulator.
Frontend publication/archives contain both variants; local Docker loading
selects the host-compatible variant. Dagger Services run
the CPU E2E directly. `mage docker:all` still exports distribution archives and
loads local tags; release runs retain that export step.
Explicit completion of an older tagged release uses its original `docker:all`
target, since that immutable source may predate `docker:check`. The source guard
outputs the compatible target alongside the validated SHA and version.

Windows artifact uploads explicitly include Windows files and test directories;
Linux executables and image tarballs are excluded. The Linux release uploader
uses an explicit allowlist of Linux binaries/images. It cannot publish Windows
payloads before native validation, even though both platforms now share `dist`.
Signed Windows publication still occurs only in `windows-native`, with the
signature uploaded last. Immutable tagged-source completion guards remain.

## Packaging evidence

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
