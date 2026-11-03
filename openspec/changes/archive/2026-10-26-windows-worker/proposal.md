## Why

Windows machines cannot currently run the complete worker from a supported native distribution. The existing Go build does not select release targets explicitly, the control interface assumes Linux paths and permissions, and media/OCR tools are supplied only by the Linux image.

## What Changes

- Add a Windows x64 portable worker bundle built with Dagger, including FFmpeg 9.0.2, ffprobe, mkvextract, the patched PgsToSrt assembly, a private .NET runtime, native OCR libraries, and pinned language data.
- Preserve the production AV1 NVENC profile, CPU SVT-AV1 and x265 alternatives, libaom, and FDK AAC.
- Resolve bundled tools independently of the current working directory and use a user-restricted Windows named pipe for runtime control.
- Select Windows updates by OS and architecture while retaining existing Linux release compatibility.
- Add native Windows tests and a Linux-server/Windows-worker end-to-end test covering download, subtitle OCR, encode, verification, upload, and local control.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `distribution`: Windows x64 native worker artifacts and pinned media/OCR toolchain.
- `configuration-and-runtime`: portable tool discovery, Windows control transport, and process shutdown.
- `self-update`: platform-specific executable selection and explicit bundle update behavior.
- `subtitles`: retain patched PGS OCR behavior in the native Windows bundle.

## Impact

Worker startup, process execution, local control, updater asset naming, Mage/Dagger build targets, CI, deployment documentation, and integration tests. The server HTTP protocol remains compatible. Windows support initially targets x64; NVIDIA tests require compatible physical hardware. FDK AAC is retained at the user's explicit request; compilation does not change upstream redistribution terms.
