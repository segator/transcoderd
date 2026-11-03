# Design

## Context

See proposal.md. The Go application already builds without cgo and transfers media over HTTP. Windows execution is partly anticipated by the command wrapper, but the local control endpoint is mandatory and Linux-specific. FFmpeg's Linux builder pins required dependency versions; PgsToSrt already publishes Windows native OCR assets.

## Goals / Non-Goals

Goals: a native x64 portable worker, production encoder parity, reproducible tool inputs, native unit and end-to-end verification, and Linux compatibility. Non-goals: a Windows server product, ARM64, GUI installer, or Windows Service integration.

## Decisions

- Cross-compile Go and the required FFmpeg libraries in Linux Dagger containers. Pin source archives and checksums to the Linux production versions. Keep Windows-specific recipe inputs separate from the established Linux builder. Validate executable capabilities on Windows rather than attempting to execute PE files on Linux.
- Verify the official FFmpeg 9.0.2 archive signature before pinning its hash. The pinned upstream Linux builder contains a stale archive hash; correct that prerequisite without changing its release version or encoder settings. Record the discrepancy in BASELINE.md.
- Use the same PgsToSrt commit, bounds patch, and language-data commit. Publish net8.0 win-x64 self-contained, preserving the package's native library directory layout. A new optional OCR executable setting avoids requiring a global dotnet host. Document the native OCR Visual C++ runtime prerequisite.
- Resolve bundled paths from os.Executable on Windows. Retain PATH and existing assembly behavior outside a portable bundle. Use argument arrays throughout; exercise paths with spaces and Unicode.
- Use go-winio named pipes with a current-user and SYSTEM ACL. Keep the Linux Unix socket transport behind an OS-specific implementation.
- Place Windows subprocesses in kill-on-close Job Objects and terminate the job on context cancellation. Verify descendants rather than only the immediate process.
- Keep the legacy Linux amd64 update name. Give Windows its own executable asset; binary updates retain media tools, while toolchain upgrades require complete bundle replacement.
- Keep the existing Linux E2E and add a native Windows worker runner against an isolated Linux server/Postgres environment. Test CPU encoding in general CI and the exact GPU profile only on compatible hardware.

## Risks / Trade-offs

- Cross-compilation failures in upstream C/C++ projects: build and exercise actual output, retain logs, and fix the recipe rather than substituting an unverified binary.
- Native OCR dependencies: test on Windows with the actual pinned assets and document required Visual C++ runtime.
- GPU availability: report CPU and GPU results separately; never infer runtime support from encoder enumeration.
- FDK AAC is explicitly retained. Its upstream nonfree redistribution limitation is unchanged.

## Migration Plan

Existing Linux builds and deployments retain their tool defaults and artifact names. Windows users extract a versioned ZIP and start the CLI with their server settings. Rollback replaces the portable folder with the previous bundle after stopping the worker.
