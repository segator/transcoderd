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
