# Native Windows worker

The Windows x64 distribution is a portable ZIP. Extract the entire archive into
a writable folder. It includes FFmpeg/ffprobe, mkvextract, the patched PgsToSrt,
a private .NET runtime, native OCR libraries and Tesseract language data. Docker,
WSL, Go and a global .NET installation are not required on the worker machine.
The native OCR libraries require the Microsoft Visual C++ 2015–2022 x64 runtime.

Start from PowerShell (the installation path may contain spaces):

```powershell
& 'C:\TranscoderD\transcoderd-worker.exe' --noUpdates `
  --web.domain 'http://server:8080' --web.token 'your-token' `
  --worker.name 'windows-gpu' --worker.maxActiveJobs 2
```

The default video profile is the same AV1 NVENC profile used by Linux. It needs a
compatible NVIDIA GPU and driver; CUDA Toolkit is not required to run it. For a
CPU worker add `--worker.ffmpegConfig.videoCodec libsvtav1`. HEVC main10 remains
available with `--worker.ffmpegConfig.videoCodec libx265`.

Tools and OCR data are located relative to the worker executable. You can launch
from any directory. `--worker.pgsConfig.executablePath` overrides the bundled OCR
executable and `--worker.pgsConfig.tessdataPath` overrides its language data. Set
the executable path to an empty value (`--worker.pgsConfig.executablePath=`) to use
the legacy dotnet/DLL settings.

Runtime control uses a named pipe accessible to the current Windows user and
SYSTEM:

```powershell
& 'C:\TranscoderD\transcoderd-worker.exe' control status
& 'C:\TranscoderD\transcoderd-worker.exe' control set-max-jobs 0
```

One worker per Windows user owns the default control endpoint. Stop the worker
with Ctrl+C. Its supervised encoder and OCR process trees are terminated during
shutdown. Schedule syntax and server API settings are shared with Linux.

Binary self-updates select `transcoderd-worker-windows-amd64.exe`. They preserve
the installed media tools. To upgrade the toolchain, stop the worker and replace
the complete bundle. `--noUpdates` keeps the extracted bundle version fixed.

Build from the repository on a Linux Dagger host:

```bash
mage build:windowsBinary # Go executable only
mage build:windows       # complete versioned ZIP in dist/
mage test:windowsBinaries # unit tests and bundle smoke-test executables
```

Cross-compilation defaults to two C/C++ compiler jobs. Set `WINDOWS_BUILD_JOBS`
to increase parallelism on larger builders.

The Windows FFmpeg recipe pins the five production encoder dependencies to the
Linux builder's versions. It does not reproduce unrelated Linux integrations
such as VAAPI or PulseAudio. FDK AAC is retained; upstream FFmpeg's nonfree
redistribution terms still apply.

Run `scripts/windows/run-tests.ps1` on Windows with `-TestDirectory`, `-Bundle`
and `-Fixture`. The bundle test performs real AV1/x265/AAC encodes and PGS OCR
with a clean PATH, so global FFmpeg and .NET installations cannot mask missing
bundle dependencies. The Windows CI workflow runs these checks on a native runner.

From WSL, test the full Windows worker against an isolated Linux server and
PostgreSQL (the bundle directory must already be extracted on the Windows disk):

```bash
WINDOWS_BUNDLE='C:\TranscoderD' mage test:windowsE2e
WINDOWS_BUNDLE='C:\TranscoderD' WINDOWS_CODEC=av1_nvenc mage test:windowsE2e
```

The E2E copies the installation and workspace into paths containing spaces and
Unicode, changes the working directory, exercises admission controls, submits the
real fixture through HTTP, and checks the completed output's codecs, duration,
resolution, languages, titles and event history. It retains native logs in the
Windows temporary directory. The NVENC run uses the default production profile.

Measured native test results and their scope are recorded in
[Windows validation](windows-worker-validation.md).

## Hosted CI and releases

Focused Windows update/administration unit tests run natively in parallel with
Linux CI, so task registration and permission failures stop before packaging.
Dagger builds every distributed binary and installer after those gates, followed
by full native media, service and upgrade validation on `windows-2025`. Dagger also builds a test-only Windows server under
`dist/windows-tests/`; it is not a distributed server product or release asset.

The native job runs all unit and bundled-tool tests and then
`scripts/windows/test-ci.ps1`. That harness creates its own PostgreSQL cluster
using the runner's `PGBIN` tools, starts a native test server, and runs the full
worker pipeline with `libsvtav1` and FDK AAC. It needs no GPU or Docker. Its source
and database are isolated; it stops its own server and database in cleanup.
Worker installation/workspace paths still contain spaces and Unicode. Logs are
uploaded even on failure. The optional WSL/NVENC test above is local hardware
validation and is not part of the hosted runner job.

When Release Please creates a release, its full version is passed to the Windows
build. Only after the native tests, tool checks, CPU E2E and mandatory updater/recovery
E2E pass does the Windows job upload `transcoderd-worker-<version>-windows-amd64.zip` and
`transcoderd-worker-windows-amd64.exe`, the complete Setup installer and signed
`windows-update.json` / `windows-update.json.sig` to the same release tag.
Portable CLI workers select the raw executable. Managed desktop installations
use the authenticated complete installer, including media tools and UI. The
release signing secret must match the pinned public verification key; signing or
verification failure prevents publishing eligible update metadata. PR runs upload CI artifacts without
publishing release assets.


The Dagger target `mage test:windowsUpdateFixtures` builds two complete test-only
installers and an isolated local release feed. `test-ci.ps1 -UpdateFixtures`
validates a SYSTEM upgrade during a real SVT-AV1 encode, immediate admission and
configuration blocking, upload completion, partial-install rollback, recovery-task
entry and automatic retry. Fixture trust keys and feed overrides are compiled
only into fixture binaries, never production builds. These destructive tests
refuse to run outside an isolated GitHub runner. See
[desktop update operations](windows-desktop.md) for the one-time bootstrap and
isolated-VM reboot drill.
