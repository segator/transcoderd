## 1. Portable runtime

- [x] 1.1 Resolve bundled media and OCR tools from the Windows executable directory and support direct OCR executable configuration; verify path and invocation unit tests.
- [x] 1.2 Implement user-restricted Windows named-pipe control while preserving Linux socket behavior; run native and Linux control tests.
- [x] 1.3 Terminate Windows subprocess trees on cancellation and select platform-specific update assets; verify native descendant termination and replacement of a running executable.

## 2. Distribution

- [x] 2.1 Add an explicit Windows Go target and a pinned Dagger FFmpeg/ffprobe cross-build retaining all required production encoders; verify the official release signature, correct the shared upstream source pin, and inspect the native executables.
- [x] 2.2 Package mkvextract, self-contained patched Windows PgsToSrt, native OCR libraries and pinned language data into a versioned portable ZIP.

## 3. Verification and documentation

- [x] 3.1 Add meaningful native Windows tests for portable paths, control, cancellation and update behavior; run Linux regression tests.
- [x] 3.2 Add and run a native Windows-worker/Linux-server E2E with real OCR and CPU encoding; validate NVENC on compatible hardware when available.
- [x] 3.3 Integrate repeatable build/test gates into CI and document deployment, prerequisites, updates, and measured validation results.
- [x] 3.4 Run strict OpenSpec validation and review the implementation against the deltas after required validation passes.

## Workflow follow-up

- Archive the change once the implementation, required tests, and spec review are complete.
