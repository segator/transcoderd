# Tasks

## 1. Runtime encoder control

- [x] 1.1 Add a dynamically adjustable FFmpeg queue limiter and verify raising/lowering does not lose jobs or interrupt active processing.
- [x] 1.2 Update `set-max-jobs` to resize the encode target and report current/target encodes; verify zero retains the last encode target.
- [x] 1.3 Keep all non-FFmpeg phase consumers at one and cover this behavior in tests.

## 2. Contract and references

- [x] 2.1 Sync runtime admission and encoding behavior into the OpenSpec and configuration reference.

## 3. Verification

- [x] 3.1 Run race-enabled targeted tests, the full Go test suite, and strict OpenSpec validation.
