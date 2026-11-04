# Proposal

## Why

A measured PR took 52 minutes even though every media dependency was cached. Three complete Windows installers consumed about 30 minutes, and a separate Linux cross-build job unnecessarily waited for Linux CI.

## What Changes

- Build Linux and Windows artifacts in the existing required `ci` job, consolidate lint there, and expose packaging timings as separate steps.
- Compile independent Windows encoder dependencies on separate Dagger branches with a shared compiler-job budget of 16 in CI; merge their installed outputs for FFmpeg.
- Measure and optimize NSIS packaging while retaining full native installation, update, rollback, OCR and encoding validation.
- Cancel superseded PR runs only, retain release/debug behavior, remove confirmed unused workflow fixtures and steps, and document the pipeline and cache boundaries.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. This is a build/tooling refactor. Existing distribution assets, languages, runtime requirements, immutable release-source checks and native validation contracts remain in force. `skip_specs: true` records this explicitly.

## Impact

GitHub Actions, Mage Windows build recipes, NSIS compiler options, build documentation and focused build/release validation. Existing application and protocol behavior is unchanged.
