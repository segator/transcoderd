# Windows worker validation

Validation dates: 2026-10-25 and 2026-10-26. The worker runs natively on Windows x64; Dagger
cross-builds the distribution on Linux. Tests use Windows build 10.0.26300.0,
with an NVIDIA RTX 5080 and driver 621.01 available for GPU verification.

The portable archive includes FFmpeg/ffprobe 9.0.2, aom 3.14.1, SVT-AV1 4.2.0,
x265 b81f650e21e8aacbe6a9ad04ce14aefc05b932c0, FDK AAC 2.0.3,
nv-codec-headers 13.1.15.0, MKVToolNix 103.0, and the same patched PgsToSrt and
language-data commits as Linux. The complete bundle is approximately 1,591 MiB
uncompressed (734 MiB ZIP).

## Native execution

`run-tests.ps1` passed all 17 worker/shared unit-test packages on Windows.
This includes current-user named-pipe control, active endpoint rejection,
cancellation of descendants in nested Windows Job Objects, cancellation before
startup, and two binary replacements while the original executable remains
mapped by a running process. Backup cleanup preserves unrelated files.

`TestWindowsBundle` passed in 1.05 seconds using the Mage-exported test binary,
with only System32 on PATH and work paths containing spaces and Unicode. It executed both FFmpeg
and ffprobe, encoded and decoded 10-bit SVT-AV1, libaom AV1 and x265, checked the
pixel format with ffprobe, encoded FDK AAC, extracted a real PGS track with
mkvextract, and converted it through the private Windows PgsToSrt runtime. OCR
produced a 260-byte English SRT with subtitle cues. Private coreclr and hostfxr
files were checked before execution. The host has the documented Visual C++ x64
runtime prerequisite installed.

## Native worker end-to-end

The native Windows worker passed `TestE2E` against an isolated Dagger Linux
server and PostgreSQL in 15.89 seconds with `libsvtav1` (preset 4, CRF 25) and
FDK AAC VBR 5. The test checked AV1/AAC/SRT output, 320x240 resolution,
source/output durations of 10.12/10.17 seconds, both audio languages, both subtitle
languages, preserved
titles, and completed Download/MKVExtract/PGS/FFMPEG/JobVerify/Upload events.
All four nonempty PGS tracks were extracted and OCRed. The installation and
workspace contained spaces and Unicode, and the worker ran from another current
directory. Named-pipe status, admission pause and resume also passed.

CPU job: `f3dbd942-d104-11f1-b725-f2baf9e0a5d9`. Native evidence directory:
`%TEMP%\TranscoderD prueba película 15b4448f59904bc7b219e0852cef690e`.

The RTX 5080 run also passed `TestE2E` in 10.12 seconds using the default
`av1_nvenc` production profile, without overriding its GPU preset or quality
settings (`p7`, `uhq`, CQ 38, full-resolution multipass, 4 B frames).
Source/output durations were 10.12/10.17 seconds; both audio tracks and
all four SRT tracks were retained. This is a functional test duration, not an
encoding throughput benchmark.

GPU job: `235f3246-d105-11f1-b2e5-ae3df8a93d0b`. Native evidence directory:
`%TEMP%\TranscoderD prueba película d9cb2635559a41828f6842afaeb5a876`.

## Regression checks

Linux `go test ./...`, race tests for command/control/steps/updater, Mage module
tests, Linux and Windows golangci-lint, actionlint, and strict OpenSpec validation
passed. Windows source verification used the official signed 9.0.2 archive. Linux uses
the upstream GitHub n9.0.2 tag archive with its own verified hash. The two archive
hashes differ; an incorrect Linux override was removed after CI exposed it.

## Scope

The integration fixture is 10.123 seconds at 320x240, with English/Spanish audio
and four nonempty PGS tracks. These checks establish fixture-level correctness;
they are not a completed-film performance benchmark or a long-running soak test.
Local raw logs and build outputs are retained under `dist/validation/`; native
per-package logs are also retained in the Windows test directory.

## Hosted-runner CPU harness

The new `scripts/windows/test-ci.ps1` harness passed locally on native Windows
with PostgreSQL 17.11 binaries, a private loopback cluster, the test-only native
server, and the worker after merging current main. It explicitly uses SVT-AV1;
no NVENC encode is attempted in hosted CI. The test checks the same full fixture
workflow, including published download checksum and server-side output validation.
It retains database/server/worker logs and stops its own processes afterward.

Native job: `38ba8dde-d114-11f1-823b-1a4b24c6dc3c`. Raw local evidence is retained
in `dist/validation/windows-ci-e2e.log` and the Windows validation directory
`%TEMP%\transcoderd-windows-validation\ci-e2e-pass`.

The fixture exposed a Windows read-only download flush failure. Its regression
passed natively and on Linux after separating download close from upload sync.
The hosted `windows-native` workflow now runs this CPU E2E after native units and
bundled-tool checks. Its Windows release upload remains after all three gates.
A release was not created or published during local validation.
