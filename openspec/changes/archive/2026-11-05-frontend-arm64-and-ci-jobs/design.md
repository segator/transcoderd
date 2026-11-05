# Design

## Context

The frontend has a static Go executable and needs CA certificates but no media tools. Its current image build forces GOARCH=amd64. Dagger 0.20.6 supports platform-specific containers and OCI platform variants for publication and archives. Existing immutable tagged-source completion may check out code predating new Mage targets. The optional debugger executes old artifacts and must retain read-only credentials.

## Goals / Non-Goals

**Goals:** Produce real amd64/arm64 frontend runtime variants, share immutable tags/aliases across them, keep ordinary CI to two execution-platform jobs, preserve all test coverage and tagged-source publication boundaries.

**Non-Goals:** Port the media toolchain/server/worker to ARM64, change frontend HTTP/auth behavior, deploy images to production or remove native Windows installation/update gates.

## Decisions

1. Cross-compile static Go frontend binaries on the native build platform. Create runtime containers with explicit linux/amd64 or linux/arm64 metadata and copy an architecture-independent CA bundle from a shared native Debian provisioning layer. This avoids emulating package installation for each build. Publication and OCI archives include both variants. Local Docker loading selects the host-compatible frontend variant.
2. Verify each runtime's ELF machine type, embedded version, nonroot identity and HTTP health; inspect a real exported OCI index for exactly the supported platforms. Foreign services use an explicit static emulator copied from the pinned Engine image into a test-only container: service init cannot rely on the automatic WithExec emulator when host binfmt is absent. Published variants/entrypoints contain no emulator. These checks are emulated, not native ARM hardware validation.
3. Resolve release source inside `ci` before switching checkouts. Compile current CI tooling once outside the source tree, so source guards and JavaScript checks remain available after checking out an older immutable release. Run guard Python/GitHub CLI and Node tests in cached native Dagger containers; retain original image build targets for old tagged source.
4. Cross-compile frontend unit-test executables along with existing Windows suites. Run them through the native test harness; retain a legacy tagged-release frontend Go-test fallback when old build recipes do not export those executables. Put the Windows network-isolation probe before artifact execution.
5. Remove normal-workflow source/PWA/unit/debug jobs. Keep native execution in `windows-native`; place artifact-only debugging in its own manual workflow with read-only token scopes and existing shared PowerShell harnesses. Its distinct validation/publication permissions justify separation and keep debug checks out of ordinary PR runs.

## Risks / Trade-offs

- Correct image metadata could hide an executable built for the wrong CPU → inspect ELF headers and execute both version/health checks.
- Consolidation could drop native frontend coverage → include it in cross-built tests and preserve the old-tag fallback.
- New tooling targets may not exist in old tags → retain current tooling outside the checkout and preserve the source guard's legacy image target.
- Debug artifact reuse could receive write credentials → retain a separate read-only manual workflow and no publication path.
- Emulation is slower than native execution → cross-compile without emulation and limit emulated commands to small runtime smoke checks.
