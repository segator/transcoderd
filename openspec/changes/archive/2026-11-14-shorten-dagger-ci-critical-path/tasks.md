## 1. CI orchestration and Dagger

- [x] 1.1 Start PR builds from the event SHA, keep release-source tests, and reuse Dagger CLI setup.
- [x] 1.2 Select full fixtures for release and manual validation and focused fixtures for ordinary builds.

## 2. Native update health

- [x] 2.1 Shorten the injected focused-fixture health failure and assert its rollback reason.

## 3. Validation

- [x] 3.1 Pass relevant Go tests, workflow lint and strict OpenSpec validation.
- [x] 3.2 Compare completed GitHub runs and validate the full fixtures.
